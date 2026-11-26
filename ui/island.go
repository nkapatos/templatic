package ui

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/markup"
	"github.com/nkapatos/templatic/internal/pagestate"
)

// Island wraps comp as a client island; tag must match the component's
// //templatic:island pragma and props is hydrated into its client module.
// Render it before Boot so Boot preloads the island's module.
func Island(tag string, props any, comp templ.Component) templ.Component {
	if !markup.ValidTag(tag) {
		panic("templatic: island tag must match [a-z0-9-]+: " + tag)
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pagestate.NoteIsland(ctx, tag)
		if _, err := fmt.Fprintf(w, `<%s data-tc-island=%q>`, tag, tag); err != nil {
			return err
		}
		if _, err := io.WriteString(w,
			`<script type="application/tc-props" data-tc-for=`+fmt.Sprintf("%q", tag)+`>`); err != nil {
			return err
		}
		if props != nil {
			if err := json.MarshalWrite(w, props, jsontext.EscapeForHTML(true)); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, `</script>`); err != nil {
			return err
		}
		if comp != nil {
			if err := comp.Render(ctx, w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, `</%s>`, tag); err != nil {
			return err
		}
		return nil
	})
}
