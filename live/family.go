package live

import (
	"fmt"
	"sync"
)

// CellFamily is a keyed set of world cells sharing a topic prefix and type; it is
// safe for concurrent use.
type CellFamily[T any] struct {
	world   *World
	prefix  string
	initial func(key string) T

	mu    sync.Mutex
	cells map[string]*Cell[T]
	// floor is the highest version a forgotten cell reached; new cells start above it so a stale event cannot outrank them.
	floor uint64
}

// NewCellFamily declares a family whose cells start at initial.
func NewCellFamily[T any](w *World, prefix string, initial T) *CellFamily[T] {
	return NewCellFamilyFunc(w, prefix, func(string) T { return initial })
}

// NewCellFamilyFunc declares a family whose cells start at initial(key).
func NewCellFamilyFunc[T any](w *World, prefix string, initial func(key string) T) *CellFamily[T] {
	validateTopic(prefix)
	if w == nil {
		panic(fmt.Errorf("live: cell family %q needs a world", prefix))
	}
	return &CellFamily[T]{world: w, prefix: prefix, initial: initial, cells: map[string]*Cell[T]{}}
}

// Topic returns the topic of key's cell, without creating it.
func (f *CellFamily[T]) Topic(key string) string { return f.prefix + "." + key }

// Get returns key's cell, declaring it on first use; it panics on an invalid key.
func (f *CellFamily[T]) Get(key string) *Cell[T] {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.cells[key]; ok {
		return c
	}
	c := NewCell(f.world, f.Topic(key), f.initial(key))
	c.mu.Lock()
	if c.version < f.floor {
		c.version = f.floor
	}
	c.mu.Unlock()
	f.cells[key] = c
	return c
}

// Keys returns the keys with a declared cell, in no particular order.
func (f *CellFamily[T]) Keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.cells))
	for k := range f.cells {
		out = append(out, k)
	}
	return out
}

// Has reports whether key's cell has been declared.
func (f *CellFamily[T]) Has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.cells[key]
	return ok
}

// Forget drops key's cell from the family and the world.
func (f *CellFamily[T]) Forget(key string) {
	f.mu.Lock()
	c, ok := f.cells[key]
	delete(f.cells, key)
	if ok {
		f.raiseFloor(c)
	}
	f.mu.Unlock()
	if ok {
		f.world.dropCell(c.name, c)
	}
}

// Clear forgets every cell of the family.
func (f *CellFamily[T]) Clear() {
	f.mu.Lock()
	cells := f.cells
	f.cells = map[string]*Cell[T]{}
	for _, c := range cells {
		f.raiseFloor(c)
	}
	f.mu.Unlock()
	for _, c := range cells {
		f.world.dropCell(c.name, c)
	}
}

func (w *World) dropCell(topic string, c cellView) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cells[topic] == c {
		delete(w.cells, topic)
	}
}

func (f *CellFamily[T]) raiseFloor(c *Cell[T]) {
	if v := c.Version(); v > f.floor {
		f.floor = v
	}
}
