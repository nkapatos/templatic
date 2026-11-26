package components

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/nkapatos/templatic/live"
)

// Book is one entry of the reading list.
type Book struct {
	ID       string
	Title    string
	Pages    int
	Shelf    string
	Favorite bool
	Tags     []string
}

// Shelves and Tags are the choices the form offers, in display order.
var (
	Shelves = []string{"to-read", "reading", "done"}
	Tags    = []string{"fiction", "history", "science"}
)

// BookIn is Save's input: the form's fields, decoded the same way from
// either transport.
type BookIn struct {
	// ID is empty for a new book. On the no-JS path the {id} route wildcard
	// fills it; the JS path sends the form's hidden field.
	ID    string `json:"id"`
	Title string `json:"title"`
	Pages int    `json:"pages"`
	Shelf string `json:"shelf"`
	// An unchecked box sends nothing, so the field stays false.
	Favorite bool `json:"favorite"`
	// One value per checked box.
	Tags []string `json:"tags"`
}

// Validate reports every refused field at once; the favorite rule names no
// field, so it shows in the form's ui.FormError slot.
func (in BookIn) Validate() error {
	var errs []error
	switch title := strings.TrimSpace(in.Title); {
	case title == "":
		errs = append(errs, live.Invalid("title", "enter a title"))
	case utf8.RuneCountInString(title) > 80:
		errs = append(errs, live.Invalid("title", "keep it under 80 characters"))
	}
	if in.Pages < 1 || in.Pages > 5000 {
		errs = append(errs, live.Invalid("pages", "between 1 and 5000"))
	}
	if !slices.Contains(Shelves, in.Shelf) {
		errs = append(errs, live.Invalid("shelf", "pick a shelf"))
	}
	for _, t := range in.Tags {
		if !slices.Contains(Tags, t) {
			errs = append(errs, live.Invalid("tags", "unknown tag "+t))
			break
		}
	}
	if in.Favorite && in.Shelf != "done" {
		errs = append(errs, live.Invalid("", "only a finished book can be a favorite"))
	}
	return errors.Join(errs...)
}

// Save creates a book (empty ID) or updates the book with ID.
var Save = live.NewCommand[BookIn]("book.save")
