package main

import (
	"slices"
	"strings"
	"sync"

	"github.com/nkapatos/templatic/live"

	"examples/tanstack/components"
)

// signups holds the usernames taken so far.
type signups struct {
	mu    sync.Mutex
	names []string
	// taken is names joined, for ui.Cell.
	taken *live.Cell[string]
}

// newSignups declares the taken-names cell on w and registers Register's JS handler.
func newSignups(w *live.World, names ...string) *signups {
	s := &signups{names: names, taken: live.NewCell(w, "signup.taken", strings.Join(names, ", "))}
	components.Register.Handle(w, func(_ *live.Session, in components.SignupIn) (any, error) {
		return nil, s.register(in)
	})
	return s
}

// register takes in.Username, refusing one that is taken; both transports call it.
func (s *signups) register(in components.SignupIn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.names, in.Username) {
		return live.Invalid("username", "that name is taken")
	}
	s.names = append(s.names, in.Username)
	s.taken.Set(strings.Join(s.names, ", "))
	return nil
}
