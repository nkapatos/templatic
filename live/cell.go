package live

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"sync"
)

// Cell is a typed, versioned value that publishes each write to its sessions.
type Cell[T any] struct {
	world   *World
	session *Session // nil: world-broadcast; set: session-scoped
	name    string

	mu      sync.Mutex
	value   T
	version uint64
}

// Get returns the current value.
func (c *Cell[T]) Get() T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// Version returns how many times the cell has been set.
func (c *Cell[T]) Version() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.version
}

// Set stores v and publishes it to subscribed sessions.
func (c *Cell[T]) Set(v T) {
	c.mu.Lock()
	c.value = v
	c.version++
	version := c.version
	c.mu.Unlock()
	c.publishValue(v, version)
}

// Update applies fn to the value under the cell's lock and publishes the result; fn
// must not call the cell's methods.
func (c *Cell[T]) Update(fn func(old T) (next T)) {
	c.mu.Lock()
	c.value = fn(c.value)
	c.version++
	v, version := c.value, c.version
	c.mu.Unlock()
	c.publishValue(v, version)
}

// Name returns the topic the cell publishes to.
func (c *Cell[T]) Name() string { return c.name }

// The caller passes the pair captured under the write lock; reading the value here would race a later Set.
func (c *Cell[T]) publishValue(v T, version uint64) {
	body, err := json.Marshal(struct {
		V T `json:"v"`
	}{V: v})
	if err != nil {
		return
	}
	ev := event{kind: KindCell, topic: c.name, body: body, version: version}
	if c.session != nil {
		c.session.offer(ev)
		return
	}
	c.world.publish(ev)
}

func (c *Cell[T]) snapshot() (body jsontext.Value, version uint64) {
	c.mu.Lock()
	v, version := c.value, c.version
	c.mu.Unlock()
	b, err := json.Marshal(struct {
		V T `json:"v"`
	}{V: v})
	if err != nil {
		return nil, 0
	}
	return b, version
}

// SessionCell declares, or returns, the typed cell topic on session s.
//
// Deprecated: use DeclareSessionCell.
func SessionCell[T any](w *World, s *Session, topic string, initial T) *Cell[T] {
	validateTopic(topic)
	if w == nil || s == nil {
		panic(fmt.Errorf("live: SessionCell needs a world and a session"))
	}
	s.cellMu.Lock()
	defer s.cellMu.Unlock()
	if existing, ok := s.cells[topic].(*Cell[T]); ok {
		return existing
	}
	if seed, ok := s.cells[topic].(*seedCell); ok {
		var v T
		if err := json.Unmarshal(seed.raw, &v); err == nil {
			initial = v
		}
	}
	c := &Cell[T]{world: w, session: s, name: topic, value: initial}
	if s.cells == nil {
		s.cells = map[string]cellView{}
	}
	s.cells[topic] = c
	return c
}

func (s *Session) snapshot() []event {
	s.cellMu.Lock()
	defer s.cellMu.Unlock()
	out := make([]event, 0, len(s.cells))
	for name, c := range s.cells {
		if body, version := c.snapshot(); body != nil {
			out = append(out, event{kind: KindCell, topic: name, body: body, version: version})
		}
	}
	return out
}

type seedCell struct{ raw jsontext.Value }

func (c *seedCell) snapshot() (jsontext.Value, uint64) {
	b := make([]byte, 0, len(c.raw)+6)
	b = append(b, `{"v":`...)
	b = append(b, c.raw...)
	return append(b, '}'), 0
}

type cellView interface {
	snapshot() (body jsontext.Value, version uint64)
}
