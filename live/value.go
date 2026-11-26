package live

import (
	"fmt"
	"sync"
)

// Value is a typed, session-scoped value that is never published.
type Value[T any] struct {
	mu sync.Mutex
	v  T
}

// Get returns the current value.
func (v *Value[T]) Get() T {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.v
}

// Set stores x.
func (v *Value[T]) Set(x T) {
	v.mu.Lock()
	v.v = x
	v.mu.Unlock()
}

// Update applies fn to the value under its lock and stores the result.
func (v *Value[T]) Update(fn func(old T) (next T)) {
	v.mu.Lock()
	v.v = fn(v.v)
	v.mu.Unlock()
}

// SessionValue declares, or returns, the server-only value under key on session s.
func SessionValue[T any](w *World, s *Session, key string, initial T) *Value[T] {
	validateTopic(key)
	if w == nil || s == nil {
		panic(fmt.Errorf("live: SessionValue needs a world and a session"))
	}
	s.cellMu.Lock()
	defer s.cellMu.Unlock()
	if existing, ok := s.values[key].(*Value[T]); ok {
		return existing
	}
	v := &Value[T]{v: initial}
	if s.values == nil {
		s.values = map[string]any{}
	}
	s.values[key] = v
	return v
}
