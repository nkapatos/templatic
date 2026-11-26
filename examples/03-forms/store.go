package main

import (
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/nkapatos/templatic/live"

	"examples/forms/components"
)

// store is the reading list, in memory and in insertion order.
type store struct {
	mu    sync.Mutex
	books []components.Book
	next  int
}

func newStore(seed ...components.Book) *store {
	s := &store{}
	for _, b := range seed {
		s.next++
		b.ID = strconv.Itoa(s.next)
		s.books = append(s.books, b)
	}
	return s
}

func (s *store) all() []components.Book {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.books)
}

func (s *store) get(id string) (components.Book, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.books, func(b components.Book) bool { return b.ID == id })
	if i < 0 {
		return components.Book{}, false
	}
	return s.books[i], true
}

// save stores in as a new book, or over the book with in.ID; an unknown ID
// is a 404 on both transports.
func (s *store) save(in components.BookIn) (components.Book, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := components.Book{
		ID:       in.ID,
		Title:    strings.TrimSpace(in.Title),
		Pages:    in.Pages,
		Shelf:    in.Shelf,
		Favorite: in.Favorite,
		Tags:     in.Tags,
	}
	if b.ID == "" {
		s.next++
		b.ID = strconv.Itoa(s.next)
		s.books = append(s.books, b)
		return b, nil
	}
	i := slices.IndexFunc(s.books, func(x components.Book) bool { return x.ID == b.ID })
	if i < 0 {
		return components.Book{}, live.NotFound("no book " + b.ID)
	}
	s.books[i] = b
	return b, nil
}
