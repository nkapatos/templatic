package live

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"maps"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// A session that cannot drain its inbox is sent a resync instead.
	inboxSize = 1024

	// The default window: events arriving within it after a burst's first event are written and flushed together.
	coalesceWindow = 8 * time.Millisecond
)

// A session touching more topics than this while detached is marked stale and resyncs on reattach.
const maxDetachedTopics = 256

// Session is one browser page connected to a world.
type Session struct {
	sid   string
	world *World

	attachMu  sync.Mutex // serializes attaches; held across the old writer's exit
	mu        sync.Mutex
	conn      *connState
	replacing *connState
	cache     map[string]event
	stale     bool // cache overflowed: reattach must resync
	timer     *time.Timer
	timerGen  uint64
	connected bool
	retired   bool
	token     string
	params    map[string]string
	req       RequestInfo
	pageURL   url.URL
	cmdReq    RequestInfo

	cellMu sync.Mutex
	cells  map[string]cellView
	subs   map[string]subscribable // collections the page shows; guarded by cellMu
	values map[string]any          // SessionValue: server-only, never enumerated
}

// Each connection has its own inbox and overflow flag so a retired one's leftovers are not mistaken for the live one's.
type connState struct {
	inbox    chan event
	pending  map[string]event // latest cell or swap per topic, slotted once in inbox; guarded by Session.mu
	overflow atomic.Bool
	cancel   context.CancelFunc
	done     chan struct{}
	doneOnce sync.Once
}

func newConn(cancel context.CancelFunc) *connState {
	return &connState{inbox: make(chan event, inboxSize), cancel: cancel, done: make(chan struct{})}
}

func (c *connState) finish() { c.doneOnce.Do(func() { close(c.done) }) }

// attachWait bounds how long attach waits for a replaced writer to exit before it resyncs.
var attachWait = 2 * time.Second

// Params returns the attested page-token params.
func (s *Session) Params() map[string]string { return maps.Clone(s.params) }

// URL returns the page address: the one the client last reported, or a later live.At the session's command returned.
func (s *Session) URL() url.URL {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pageURL
}

// Accepts only a plain same-origin path.
func (s *Session) setReportedURL(raw string) {
	if raw == "" || len(raw) > 2048 || raw[0] != '/' || (len(raw) > 1 && (raw[1] == '/' || raw[1] == '\\')) {
		return
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return
	}
	s.mu.Lock()
	s.pageURL = *u
	s.mu.Unlock()
}

// SID returns the session identifier.
func (s *Session) SID() string { return s.sid }

// A detached session keeps only the latest swap per topic: reattach resends cells and catches collections up by version.
func (s *Session) offer(ev event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retired {
		return
	}
	c := s.conn
	if c == nil {
		if ev.kind == KindSwap {
			s.stashLocked(ev)
		}
		return
	}
	// Latest-wins kinds take one inbox slot per topic, so a write stall fills it with topics, not with writes.
	if latestWins(ev.kind) {
		key := latestKey(ev)
		if p, ok := c.pending[key]; ok {
			if ev.version >= p.version {
				c.pending[key] = ev
			}
			return
		}
		if c.pending == nil {
			c.pending = map[string]event{}
		}
		c.pending[key] = ev
	}
	select {
	case c.inbox <- ev:
	default:
		c.overflow.Store(true)
		c.cancel()
	}
}

func latestWins(kind string) bool { return kind == KindCell || kind == KindSwap }

func latestKey(ev event) string { return ev.topic + "\x00" + ev.kind }

// takeLocked swaps an inbox slot for the latest event of its topic.
func (c *connState) takeLocked(ev event) event {
	if !latestWins(ev.kind) {
		return ev
	}
	key := latestKey(ev)
	if p, ok := c.pending[key]; ok {
		delete(c.pending, key)
		return p
	}
	return ev
}

func (s *Session) take(c *connState, ev event) event {
	if !latestWins(ev.kind) {
		return ev
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return c.takeLocked(ev)
}

// latestFilter tracks what one connection has sent: publishes can reach the inbox out of order, so per topic a
// cell or swap older than the last one sent is dropped.
type latestFilter map[string]uint64

func (f latestFilter) newer(ev event) bool {
	if !latestWins(ev.kind) {
		return true
	}
	key := latestKey(ev)
	if last, ok := f[key]; ok && ev.version <= last {
		return false
	}
	f[key] = ev.version
	return true
}

func (s *Session) stashLocked(ev event) {
	if prev, ok := s.cache[ev.topic]; ok {
		if ev.version >= prev.version {
			s.cache[ev.topic] = ev
		}
		return
	}
	if len(s.cache) >= maxDetachedTopics {
		s.stale = true
		return
	}
	if s.cache == nil {
		s.cache = map[string]event{}
	}
	s.cache[ev.topic] = ev
}

// The wait holds no lock: the old writer's requeue takes s.mu. Returns cached swaps oldest first; stale means resync.
func (s *Session) attach(c *connState) (cached []event, stale, resumed, retired bool) {
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	s.mu.Lock()
	if s.retired {
		s.mu.Unlock()
		return nil, false, false, true
	}
	old := s.conn
	if old != nil {
		s.replacing = old
		old.cancel()
		resumed = true
	}
	s.mu.Unlock()
	timedOut := false
	if old != nil {
		t := time.NewTimer(attachWait)
		select {
		case <-old.done:
		case <-t.C:
			timedOut = true
		}
		t.Stop()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replacing = nil
	if s.retired {
		return nil, false, false, true
	}
	if old != nil {
		s.drainLocked(old)
		if timedOut || old.overflow.Load() {
			s.stale = true
		}
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
		s.timerGen++
		resumed = true
	}
	s.conn = c
	for _, ev := range s.cache {
		cached = append(cached, ev)
	}
	slices.SortFunc(cached, func(a, b event) int { return cmp.Compare(a.version, b.version) })
	s.cache = nil
	stale = s.stale
	return cached, stale, resumed, false
}

func (s *Session) drainLocked(c *connState) {
	for {
		select {
		case ev := <-c.inbox:
			if ev = c.takeLocked(ev); ev.kind == KindSwap {
				s.stashLocked(ev)
			}
		default:
			return
		}
	}
}

func (s *Session) requeue(c *connState, evs []event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (s.conn != c && s.replacing != c) || s.retired {
		return
	}
	for _, ev := range evs {
		if ev.kind == KindSwap {
			s.stashLocked(ev)
		}
	}
}

// Retires at once with no grace window or after overflow, since that client resyncs anyway.
func (s *Session) detach(c *connState) {
	s.mu.Lock()
	if s.conn != c || s.replacing == c {
		s.mu.Unlock()
		return
	}
	s.conn = nil
	s.drainLocked(c)
	grace := s.world.resumeGrace()
	if grace <= 0 || c.overflow.Load() || s.retired {
		s.mu.Unlock()
		s.retire()
		return
	}
	s.timerGen++
	gen := s.timerGen
	s.timer = time.AfterFunc(grace, func() { s.expire(gen) })
	s.mu.Unlock()
}

func (s *Session) expire(gen uint64) {
	s.mu.Lock()
	if s.conn != nil || s.timerGen != gen || s.retired {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.retire()
}

// Fires OnDisconnect exactly once, after the registry no longer holds the session.
func (s *Session) retire() {
	s.mu.Lock()
	if s.retired {
		s.mu.Unlock()
		return
	}
	s.retired = true
	s.cache = nil
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		conn.cancel()
	}
	s.cellMu.Lock()
	subs := s.subs
	s.subs = nil
	s.cellMu.Unlock()
	for _, c := range subs {
		c.unsubscribe(s)
	}
	w := s.world
	w.mu.Lock()
	if w.sessions[s.sid] == s {
		delete(w.sessions, s.sid)
	}
	fns := slices.Clone(w.onDisconnect)
	w.mu.Unlock()
	for _, fn := range fns {
		fn(s)
	}
}

func (s *Session) isRetired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retired
}

// Attached reports whether the session currently has a live connection.
func (s *Session) Attached() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn != nil
}

func (s *Session) loop(ctx context.Context, c *connState, prelude []event, w io.Writer, flush func()) error {
	defer c.finish()
	seq := uint64(0)
	// A write to a vanished client fails and ends the loop; otherwise a silent disconnect keeps the session registered.
	heartbeat := time.NewTicker(s.world.heartbeatInterval())
	defer heartbeat.Stop()
	keepalive := func() error {
		if _, err := io.WriteString(w, ": hb\n\n"); err != nil {
			return err
		}
		flush()
		return nil
	}
	frame := func(kind, topic string, body []byte) error {
		seq++
		line := fmt.Appendf(nil, "id: %d\nevent: tc\ndata: {\"kind\":\"%s\"", seq, kind)
		if topic != "" {
			line = append(line, fmt.Appendf(nil, ",\"topic\":\"%s\"", topic)...)
		}
		if len(body) > 0 {
			line = append(line, ',')
			line = append(line, body[1:]...)
		} else {
			line = append(line, '}')
		}
		line = append(line, '\n', '\n')
		_, err := w.Write(line)
		return err
	}

	newer := latestFilter{}.newer

	initial := append(s.world.snapshot(), s.snapshot()...)
	initial = append(initial, prelude...)
	for i, ev := range initial {
		if !newer(ev) {
			continue
		}
		if err := frame(ev.kind, ev.topic, ev.body); err != nil {
			s.requeue(c, initial[i:])
			return err
		}
	}
	if err := frame(KindReady, "", nil); err != nil {
		return err
	}
	flush()

	for {
		select {
		case <-ctx.Done():
			if c.overflow.Load() {
				_ = frame(KindResync, "", nil)
				flush()
			}
			return nil
		case <-heartbeat.C:
			if err := keepalive(); err != nil {
				return err
			}
		case ev := <-c.inbox:
			out := coalescer{}
			out.add(s.take(c, ev))
			timer := time.NewTimer(s.world.coalesceWindowOf())
		drain:
			for {
				select {
				case more := <-c.inbox:
					out.add(s.take(c, more))
					if out.n >= inboxSize {
						break drain
					}
				case <-timer.C:
					break drain
				case <-ctx.Done():
					// Write the resync here: overflow cancels the context, and the next inbox read could lose it.
					if c.overflow.Load() {
						_ = frame(KindResync, "", nil)
						flush()
					}
					timer.Stop()
					s.requeue(c, out.events())
					return nil
				}
			}
			timer.Stop()
			if c.overflow.Load() {
				_ = frame(KindResync, "", nil)
				flush()
				return nil
			}
			batch := out.events()
			for i, fev := range batch {
				if !newer(fev) {
					continue
				}
				if err := frame(fev.kind, fev.topic, fev.body); err != nil {
					s.requeue(c, batch[i:])
					return err
				}
			}
			flush()
		}
	}
}

// coalescer gathers a burst: cells and swaps are latest-wins per topic, in the slot of their first arrival; ops
// and resets keep arrival order, so none is lost or reordered against another of its collection.
type coalescer struct {
	order   []event
	byTopic map[string]int
	n       int
}

func (c *coalescer) add(ev event) {
	c.n++
	if ev.kind != KindCell && ev.kind != KindSwap {
		c.order = append(c.order, ev)
		return
	}
	if c.byTopic == nil {
		c.byTopic = map[string]int{}
	}
	key := ev.topic + "\x00" + ev.kind
	if i, ok := c.byTopic[key]; ok {
		// offers can land out of order; keep the highest version
		if ev.version >= c.order[i].version {
			c.order[i] = ev
		}
		return
	}
	c.byTopic[key] = len(c.order)
	c.order = append(c.order, ev)
}

func (c *coalescer) events() []event { return c.order }

// subscribe makes s receive the ops of each collection in held (topic → the version the page holds), with what the
// page lacks first; unknown topics are ignored.
func (s *Session) subscribe(held map[string]int64) error {
	var first error
	for topic, ver := range held {
		s.world.mu.RLock()
		c, ok := s.world.collections[topic]
		s.world.mu.RUnlock()
		if !ok {
			continue
		}
		s.cellMu.Lock()
		if s.subs == nil {
			s.subs = map[string]subscribable{}
		}
		s.subs[topic] = c
		s.cellMu.Unlock()
		if err := c.subscribe(s, ver); err != nil && first == nil {
			first = err
		}
	}
	return first
}
