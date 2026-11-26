package ui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/markup"
	"github.com/nkapatos/templatic/live"
)

// Cell renders the value of c in a marker element the runtime updates in place.
func Cell[T any](c *live.Cell[T]) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		v := c.Get()
		if _, err := fmt.Fprintf(w,
			`<span data-tc-cell=%q>%s</span>`,
			c.Name(), templ.EscapeString(markup.RenderCellValue(v))); err != nil {
			return err
		}
		return nil
	})
}

// SessionCell renders a session-scoped cell topic painted with initial.
//
// Deprecated: use Bind.
func SessionCell(topic string, initial any) templ.Component {
	if !markup.ValidName(topic) {
		panic("templatic: cell topic must match [A-Za-z0-9_.-]+: " + topic)
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := fmt.Fprintf(w,
			`<span data-tc-cell=%q>%s</span>`,
			topic, templ.EscapeString(markup.RenderCellValue(initial)))
		return err
	})
}

// cellAttrPrefix prefixes an attribute binding's marker, data-tc-a-{attr}.
const cellAttrPrefix = "data-tc-a-"

// checkBindAttr panics unless attr is data-*, aria-*, class or value, and not
// a runtime data-tc-* name.
func checkBindAttr(attr string) {
	a := strings.ToLower(attr)
	ok := a == "class" || a == "value" ||
		(strings.HasPrefix(a, "data-") && len(a) > len("data-") && !strings.HasPrefix(a, "data-tc-")) ||
		(strings.HasPrefix(a, "aria-") && len(a) > len("aria-"))
	if ok {
		for _, r := range a {
			if !(r == '-' || r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
				ok = false
				break
			}
		}
	}
	if !ok {
		panic("templatic: a cell can bind only data-*, aria-*, class or value, not " + attr)
	}
}

func cellAttrs(topic, attr string, v any) templ.Attributes {
	a := strings.ToLower(attr)
	out := templ.Attributes{"data-tc-a": "", cellAttrPrefix + a: topic}
	if s := markup.RenderCellValue(v); s != "" {
		out[a] = s
	}
	return out
}

// CellAttr binds c to an attribute of the element it is spread on; an empty or
// null value removes the attribute. It panics on an attribute other than
// data-*, aria-*, class or value.
func CellAttr[T any](c *live.Cell[T], attr string) templ.Attributes {
	checkBindAttr(attr)
	return cellAttrs(c.Name(), attr, c.Get())
}

// SessionCellAttr is CellAttr for a session-scoped topic painted with initial.
//
// Deprecated: use BindAttr.
func SessionCellAttr(topic, attr string, initial any) templ.Attributes {
	if !markup.ValidName(topic) {
		panic("templatic: cell topic must match [A-Za-z0-9_.-]+: " + topic)
	}
	checkBindAttr(attr)
	return cellAttrs(topic, attr, initial)
}

// Bind renders the marker of a declared session cell painted with v.
func Bind[T any](cell *live.SessionCellDecl[T], v T) templ.Component {
	return SessionCell(cell.Topic(), v)
}

// BindAttr binds a declared session cell to an attribute, painted with v.
func BindAttr[T any](cell *live.SessionCellDecl[T], attr string, v T) templ.Attributes {
	return SessionCellAttr(cell.Topic(), attr, v)
}
