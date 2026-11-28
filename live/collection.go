package live

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/a-h/templ"
	templruntime "github.com/a-h/templ/runtime"

	"github.com/nkapatos/templatic/internal/markup"
)

// DefaultHistory is how many bytes of recent ops a collection keeps to catch a page up.
const DefaultHistory = 64 << 10

// Collection is a keyed, ordered set of items the world holds; each change is published once, as a versioned op, to
// the sessions whose page shows it. With Renders its ops carry each item's HTML for the container Render writes;
// without, they carry the item as JSON for the islands bound to its topic through Snapshot. Configure it before
// first use.
type Collection[T any] struct {
	world     *World
	topic     string
	key       func(T) string
	render    func(T) templ.Component
	attrs     func(T) templ.Attributes
	container string
	item      string
	tail      bool
	limit     int
	history   int

	mu      sync.Mutex
	order   []string // oldest first; a head collection displays it reversed
	items   map[string]T
	version uint64
	ring    opRing
	subs    map[*Session]struct{}
	reset   struct {
		version uint64
		body    jsontext.Value
	}
}

// NewCollection declares, or returns, the collection under topic on w, its items keyed by key.
func NewCollection[T any](w *World, topic string, key func(T) string) *Collection[T] {
	validateTopic(topic)
	if w == nil || key == nil {
		panic(fmt.Errorf("live: collection %q needs a world and a key func", topic))
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if existing, ok := w.collections[topic].(*Collection[T]); ok {
		return existing
	}
	c := &Collection[T]{
		world: w, topic: topic, key: key, container: "div", item: "div", history: DefaultHistory,
		items: map[string]T{}, subs: map[*Session]struct{}{},
	}
	w.collections[topic] = c
	return c
}

// Renders sets the item body renderer, making the collection server-rendered; the item's root element and key come
// from the collection.
func (c *Collection[T]) Renders(fn func(T) templ.Component) *Collection[T] { c.render = fn; return c }

// Elements sets the container and item element names, such as "tbody" and "tr" (default "div" and "div").
func (c *Collection[T]) Elements(container, item string) *Collection[T] {
	if !validElement(container) || !validElement(item) {
		panic(fmt.Errorf("live: collection %q: element names must match [a-z][a-z0-9-]*", c.topic))
	}
	c.container, c.item = container, item
	return c
}

// ItemAttrs sets the attributes of each item's root element.
func (c *Collection[T]) ItemAttrs(fn func(T) templ.Attributes) *Collection[T] { c.attrs = fn; return c }

// Tail puts new items after the existing ones instead of before them.
func (c *Collection[T]) Tail() *Collection[T] { c.tail = true; return c }

// Limit keeps at most n items, dropping the oldest; n <= 0 means no limit.
func (c *Collection[T]) Limit(n int) *Collection[T] { c.limit = max(n, 0); return c }

// History sets how many bytes of recent ops are kept to catch a page up (default DefaultHistory); past it, a page
// gets the whole set.
func (c *Collection[T]) History(bytes int) *Collection[T] { c.history = max(bytes, 0); return c }

// Topic returns the collection's topic.
func (c *Collection[T]) Topic() string { return c.topic }

// Version returns how many changes the collection has published.
func (c *Collection[T]) Version() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.version
}

// Get returns the item under key.
func (c *Collection[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	return it, ok
}

// Items returns the items in display order.
func (c *Collection[T]) Items() []T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.itemsLocked()
}

// Len returns the number of items.
func (c *Collection[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.order)
}

func (c *Collection[T]) itemsLocked() []T {
	out := make([]T, len(c.order))
	for i, k := range c.order {
		out[c.displayIndex(i)] = c.items[k]
	}
	return out
}

// displayIndex maps a position in c.order to its position on the page.
func (c *Collection[T]) displayIndex(i int) int {
	if c.tail {
		return i
	}
	return len(c.order) - 1 - i
}

// Upsert inserts each item whose key is new and replaces each one whose key is held, publishing one op per item.
// A key outside the topic grammar or a failing render publishes nothing.
func (c *Collection[T]) Upsert(items ...T) error {
	if len(items) == 0 {
		return nil
	}
	keys := make([]string, len(items))
	ops := make([]jsontext.Value, len(items))
	for i, it := range items {
		k := c.key(it)
		if err := checkTopic(k); err != nil {
			return fmt.Errorf("live: collection %q: item key %q: %w", c.topic, k, err)
		}
		op, err := c.upsertOp(it, k)
		if err != nil {
			return err
		}
		keys[i], ops[i] = k, op
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, it := range items {
		k := keys[i]
		if _, held := c.items[k]; !held {
			c.order = append(c.order, k)
		}
		c.items[k] = it
		c.trimLocked()
	}
	c.publishLocked(ops)
	return nil
}

// Update applies fn to the item under key and publishes the result as one upsert, all under the collection's lock;
// it reports whether key was held. fn must keep the item's key and must not call the collection's methods.
func (c *Collection[T]) Update(key string, fn func(old T) T) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	old, ok := c.items[key]
	if !ok {
		return false, nil
	}
	next := fn(old)
	if k := c.key(next); k != key {
		return true, fmt.Errorf("live: collection %q: Update changed key %q to %q", c.topic, key, k)
	}
	op, err := c.upsertOp(next, key)
	if err != nil {
		return true, err
	}
	c.items[key] = next
	c.publishLocked([]jsontext.Value{op})
	return true, nil
}

// upsertOp is {"op":"u","k":key} with the item's HTML, or with its JSON for a data collection.
func (c *Collection[T]) upsertOp(it T, key string) (jsontext.Value, error) {
	if c.render == nil {
		v, err := json.Marshal(it)
		if err != nil {
			return nil, fmt.Errorf("live: collection %q: item %q: %w", c.topic, key, err)
		}
		return json.Marshal(struct {
			Op string         `json:"op"`
			K  string         `json:"k"`
			V  jsontext.Value `json:"v"`
		}{"u", key, v})
	}
	html, err := c.renderItem(it, key)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Op   string `json:"op"`
		K    string `json:"k"`
		HTML string `json:"html"`
	}{"u", key, html})
}

// Remove drops the items under keys, publishing one op per key held; keys it does not hold are ignored.
func (c *Collection[T]) Remove(keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var ops []jsontext.Value
	for _, k := range keys {
		if _, held := c.items[k]; !held {
			continue
		}
		delete(c.items, k)
		c.order = slices.DeleteFunc(c.order, func(o string) bool { return o == k })
		op, err := json.Marshal(struct {
			Op string `json:"op"`
			K  string `json:"k"`
		}{"d", k})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	c.publishLocked(ops)
	return nil
}

// Reset replaces every item with items, given in display order, and sends each subscribed page the whole set.
func (c *Collection[T]) Reset(items []T) error {
	order := make([]string, 0, len(items))
	byKey := make(map[string]T, len(items))
	for _, it := range items {
		k := c.key(it)
		if err := checkTopic(k); err != nil {
			return fmt.Errorf("live: collection %q: item key %q: %w", c.topic, k, err)
		}
		if _, dup := byKey[k]; dup {
			return fmt.Errorf("live: collection %q: duplicate item key %q", c.topic, k)
		}
		order = append(order, k)
		byKey[k] = it
	}
	if !c.tail {
		slices.Reverse(order)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order, c.items = order, byKey
	c.trimLocked()
	c.version++
	// ops from before a reset cannot be applied after it
	c.ring.clear()
	body, err := c.resetBodyLocked()
	if err != nil {
		return err
	}
	for s := range c.subs {
		s.offer(event{kind: KindReset, topic: c.topic, body: body, version: c.version})
	}
	return nil
}

// Render writes the container with the current items and the version they are at.
func (c *Collection[T]) Render(ctx context.Context, w io.Writer) error { return c.render0(ctx, w, nil) }

// With renders the container with attrs added to it, such as a class; the collection's own attributes win.
func (c *Collection[T]) With(attrs templ.Attributes) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error { return c.render0(ctx, w, attrs) })
}

func (c *Collection[T]) render0(ctx context.Context, w io.Writer, attrs templ.Attributes) error {
	if c.render == nil {
		panic(fmt.Errorf("live: collection %q has no Renders: bind it to an island through Snapshot instead", c.topic))
	}
	c.mu.Lock()
	keys := make([]string, len(c.order))
	for i, k := range c.order {
		keys[c.displayIndex(i)] = k
	}
	items, version := c.itemsLocked(), c.version
	c.mu.Unlock()
	open := c.openTag(version)
	if _, err := io.WriteString(w, open[:len(open)-1]); err != nil {
		return err
	}
	if len(attrs) > 0 {
		extra := maps.Clone(attrs)
		for k := range extra {
			if strings.HasPrefix(k, "data-tc-") {
				delete(extra, k)
			}
		}
		if err := templruntime.RenderAttributes(ctx, w, io.Discard, c.container, extra); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, ">"); err != nil {
		return err
	}
	for i, it := range items {
		if err := c.writeItem(ctx, w, it, keys[i]); err != nil {
			return err
		}
	}
	return markup.WriteStrings(w, "</", c.container, ">")
}

func (c *Collection[T]) openTag(version uint64) string {
	var b strings.Builder
	b.WriteString("<")
	b.WriteString(c.container)
	b.WriteString(` data-tc-collection="`)
	b.WriteString(c.topic)
	b.WriteString(`" data-tc-ver="`)
	b.WriteString(strconv.FormatUint(version, 10))
	b.WriteString(`"`)
	if c.tail {
		b.WriteString(` data-tc-dir="tail"`)
	}
	if c.limit > 0 {
		b.WriteString(` data-tc-limit="`)
		b.WriteString(strconv.Itoa(c.limit))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	return b.String()
}

func (c *Collection[T]) writeItem(ctx context.Context, w io.Writer, it T, key string) error {
	if c.render == nil {
		panic(fmt.Errorf("live: collection %q needs Renders before use", c.topic))
	}
	if err := markup.WriteStrings(w, "<", c.item, ` data-tc-item="`, key, `"`); err != nil {
		return err
	}
	if c.attrs != nil {
		attrs := c.attrs(it)
		if _, ok := attrs["data-tc-item"]; ok {
			// the key is the collection's; an attribute cannot move an item to another key
			attrs = maps.Clone(attrs)
			delete(attrs, "data-tc-item")
		}
		// templ's own sanitizing; script handlers have no place to go before a live-inserted item, so they drop
		if err := templruntime.RenderAttributes(ctx, w, io.Discard, c.item, attrs); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, ">"); err != nil {
		return err
	}
	if body := c.render(it); body != nil {
		if err := body.Render(ctx, w); err != nil {
			return err
		}
	}
	return markup.WriteStrings(w, "</", c.item, ">")
}

func (c *Collection[T]) renderItem(it T, key string) (string, error) {
	var buf bytes.Buffer
	if err := c.writeItem(context.Background(), &buf, it, key); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// The far end holds the oldest items; the page trims by the same rule, so a trim is not an op.
func (c *Collection[T]) trimLocked() {
	if c.limit <= 0 || len(c.order) <= c.limit {
		return
	}
	excess := len(c.order) - c.limit
	for _, k := range c.order[:excess] {
		delete(c.items, k)
	}
	// re-slicing is O(1); append's next growth copies only the live part
	c.order = c.order[excess:]
}

// Ops are numbered and offered under c.mu, so every session receives them in version order.
func (c *Collection[T]) publishLocked(ops []jsontext.Value) {
	if len(ops) == 0 {
		return
	}
	for _, op := range ops {
		c.version++
		c.ring.add(c.version, op, c.history)
	}
	body := opsBody(c.version, ops)
	for s := range c.subs {
		s.offer(event{kind: KindOps, topic: c.topic, body: body, version: c.version})
	}
}

func opsBody(version uint64, ops []jsontext.Value) jsontext.Value {
	n := 32
	for _, op := range ops {
		n += len(op) + 1
	}
	b := make([]byte, 0, n)
	b = append(b, `{"ver":`...)
	b = strconv.AppendUint(b, version, 10)
	b = append(b, `,"ops":[`...)
	for i, op := range ops {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, op...)
	}
	return append(b, "]}"...)
}

// Built under c.mu so no op can land between the set it holds and the ops after it; cached per version, so pages
// reconnecting together cost one build.
func (c *Collection[T]) resetBodyLocked() (jsontext.Value, error) {
	if c.reset.body != nil && c.reset.version == c.version {
		return c.reset.body, nil
	}
	if c.render == nil {
		body, err := c.snapshotLocked().MarshalJSON()
		if err != nil {
			return nil, err
		}
		c.reset.version, c.reset.body = c.version, body
		return body, nil
	}
	var buf bytes.Buffer
	for i := range c.order {
		k := c.order[c.displayIndex(i)]
		if err := c.writeItem(context.Background(), &buf, c.items[k], k); err != nil {
			return nil, err
		}
	}
	body, err := json.Marshal(struct {
		Ver  uint64 `json:"ver"`
		HTML string `json:"html"`
	}{c.version, buf.String()})
	if err != nil {
		return nil, err
	}
	c.reset.version, c.reset.body = c.version, body
	return body, nil
}

// Snapshot returns the items and the version they are at, read together, for an island prop bound to the
// collection's topic; it panics on a collection with Renders, whose ops carry HTML.
func (c *Collection[T]) Snapshot() Snapshot[T] {
	if c.render != nil {
		panic(fmt.Errorf("live: collection %q has Renders: render it with Render instead of binding a Snapshot", c.topic))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

func (c *Collection[T]) snapshotLocked() Snapshot[T] {
	keys := make([]string, len(c.order))
	for i, k := range c.order {
		keys[c.displayIndex(i)] = k
	}
	return Snapshot[T]{version: c.version, keys: keys, items: c.itemsLocked(), tail: c.tail, limit: c.limit}
}

// Snapshot is a data collection's items, in display order, at one version. As an island props field bound to the
// collection's topic it seeds the page, which then receives the ops after that version; the field's TypeScript type
// is the items' array.
type Snapshot[T any] struct {
	version uint64
	keys    []string
	items   []T
	tail    bool
	limit   int
}

// Version returns the version the items are at.
func (s Snapshot[T]) Version() uint64 { return s.version }

// Items returns the items in display order.
func (s Snapshot[T]) Items() []T { return s.items }

// MarshalJSON writes {"tc":"snapshot","ver","tail","limit","items":[{"k","v"}…]}: the page needs the keys to apply
// ops and the order rules to place and trim inserts.
func (s Snapshot[T]) MarshalJSON() ([]byte, error) {
	type entry struct {
		K string `json:"k"`
		V T      `json:"v"`
	}
	entries := make([]entry, len(s.items))
	for i, it := range s.items {
		entries[i] = entry{s.keys[i], it}
	}
	return json.Marshal(struct {
		TC    string  `json:"tc"`
		Ver   uint64  `json:"ver"`
		Tail  bool    `json:"tail,omitzero"`
		Limit int     `json:"limit,omitzero"`
		Items []entry `json:"items"`
	}{"snapshot", s.version, s.tail, s.limit, entries})
}

// subscribe makes s receive the collection's ops, first sending what a page at version held lacks: the ops since
// then while the history reaches back, else the whole set. held < 0 means the page holds no known version.
func (c *Collection[T]) subscribe(s *Session, held int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subs[s] = struct{}{}
	switch {
	case held >= 0 && uint64(held) == c.version:
		return nil
	case held >= 0 && uint64(held) < c.version:
		if ops, ok := c.ring.since(uint64(held)); ok {
			s.offer(event{kind: KindOps, topic: c.topic, body: opsBody(c.version, ops), version: c.version})
			return nil
		}
	}
	body, err := c.resetBodyLocked()
	if err != nil {
		return err
	}
	s.offer(event{kind: KindReset, topic: c.topic, body: body, version: c.version})
	return nil
}

func (c *Collection[T]) unsubscribe(s *Session) {
	c.mu.Lock()
	delete(c.subs, s)
	c.mu.Unlock()
}

// subscribable is the type-erased side of a collection that sessions and the world hold.
type subscribable interface {
	subscribe(s *Session, held int64) error
	unsubscribe(s *Session)
}

// opRing is a circular buffer of the most recent ops within a byte budget. Versions in it are contiguous, so the op
// at version v sits at offset v-first from head: finding where a catch-up starts is arithmetic, and adding or dropping
// an op never copies. The buffer doubles only while the budget allows more entries than it holds.
type opRing struct {
	buf   []jsontext.Value
	head  int    // index of the oldest op
	n     int    // ops held
	first uint64 // version of the oldest op
	bytes int
}

func (r *opRing) at(i int) int { return (r.head + i) & (len(r.buf) - 1) }

func (r *opRing) add(version uint64, op jsontext.Value, budget int) {
	if r.n == 0 {
		r.first = version
	}
	if r.n == len(r.buf) {
		r.grow()
	}
	r.buf[r.at(r.n)] = op
	r.n++
	r.bytes += len(op)
	for r.bytes > budget && r.n > 0 {
		r.bytes -= len(r.buf[r.head])
		r.buf[r.head] = nil // release the op
		r.head = r.at(1)
		r.n--
		r.first++
	}
}

// grow doubles the buffer (a power of two, so at can mask), unrolling the ops to start at index 0.
func (r *opRing) grow() {
	next := make([]jsontext.Value, max(16, 2*len(r.buf)))
	for i := range r.n {
		next[i] = r.buf[r.at(i)]
	}
	r.buf, r.head = next, 0
}

func (r *opRing) clear() { *r = opRing{} }

// since returns the ops after version, if the ring still holds every one of them.
func (r *opRing) since(version uint64) ([]jsontext.Value, bool) {
	if r.n == 0 || version+1 < r.first {
		return nil, false
	}
	from := int(version + 1 - r.first)
	if from >= r.n {
		return nil, true
	}
	out := make([]jsontext.Value, 0, r.n-from)
	for i := from; i < r.n; i++ {
		out = append(out, r.buf[r.at(i)])
	}
	return out, true
}

func validElement(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// CollectionFamily is a collection per entity, such as a thread per conversation, under one topic prefix; its members
// share their configuration. Configure it before the first Get.
type CollectionFamily[T any] struct {
	world *World
	proto Collection[T]

	mu      sync.Mutex
	members map[string]*Collection[T]
	// floor is the highest version a forgotten member reached; a new one starts above it, so a page still holding
	// the old member's version can never look current.
	floor uint64
}

// NewCollectionFamily declares a family of collections under prefix, their items keyed by key.
func NewCollectionFamily[T any](w *World, prefix string, key func(T) string) *CollectionFamily[T] {
	validateTopic(prefix)
	if w == nil || key == nil {
		panic(fmt.Errorf("live: collection family %q needs a world and a key func", prefix))
	}
	return &CollectionFamily[T]{
		world:   w,
		proto:   Collection[T]{topic: prefix, key: key, container: "div", item: "div", history: DefaultHistory},
		members: map[string]*Collection[T]{},
	}
}

// Renders sets the members' item body renderer.
func (f *CollectionFamily[T]) Renders(fn func(T) templ.Component) *CollectionFamily[T] {
	f.proto.render = fn
	return f
}

// Elements sets the members' container and item element names.
func (f *CollectionFamily[T]) Elements(container, item string) *CollectionFamily[T] {
	f.proto.Elements(container, item)
	return f
}

// ItemAttrs sets the attributes of the members' item root elements.
func (f *CollectionFamily[T]) ItemAttrs(fn func(T) templ.Attributes) *CollectionFamily[T] {
	f.proto.attrs = fn
	return f
}

// Tail makes the members put new items after the existing ones.
func (f *CollectionFamily[T]) Tail() *CollectionFamily[T] { f.proto.tail = true; return f }

// Limit keeps at most n items per member.
func (f *CollectionFamily[T]) Limit(n int) *CollectionFamily[T] { f.proto.limit = max(n, 0); return f }

// History sets each member's catch-up budget in bytes.
func (f *CollectionFamily[T]) History(bytes int) *CollectionFamily[T] {
	f.proto.history = max(bytes, 0)
	return f
}

// Topic returns the member topic for id, without creating it.
func (f *CollectionFamily[T]) Topic(id string) string { return f.proto.topic + "." + id }

// Get returns id's member, declaring it on first use; an id outside the topic grammar is an error, since ids come
// from data.
func (f *CollectionFamily[T]) Get(id string) (*Collection[T], error) {
	if err := checkTopic(id); err != nil {
		return nil, fmt.Errorf("live: collection family %q: member %q: %w", f.proto.topic, id, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.members[id]; ok {
		return c, nil
	}
	c := NewCollection(f.world, f.Topic(id), f.proto.key)
	c.render, c.attrs, c.container, c.item = f.proto.render, f.proto.attrs, f.proto.container, f.proto.item
	c.tail, c.limit, c.history = f.proto.tail, f.proto.limit, f.proto.history
	c.mu.Lock()
	c.version = max(c.version, f.floor)
	c.mu.Unlock()
	f.members[id] = c
	return c, nil
}

// Forget drops id's member from the family and the world; pages still showing it stop receiving ops.
func (f *CollectionFamily[T]) Forget(id string) {
	f.mu.Lock()
	c, ok := f.members[id]
	delete(f.members, id)
	if ok {
		f.floor = max(f.floor, c.Version())
	}
	f.mu.Unlock()
	if !ok {
		return
	}
	f.world.mu.Lock()
	if f.world.collections[c.topic] == subscribable(c) {
		delete(f.world.collections, c.topic)
	}
	f.world.mu.Unlock()
	c.mu.Lock()
	subs := c.subs
	c.subs = map[*Session]struct{}{}
	c.mu.Unlock()
	for s := range subs {
		s.cellMu.Lock()
		delete(s.subs, c.topic)
		s.cellMu.Unlock()
	}
}
