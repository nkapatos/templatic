package ui

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/internal/markup"
)

// Region marks its children as the live region published under topic, with
// the page supplying the initial render.
func Region(topic string, comp templ.Component, opts ...Option) templ.Component {
	if !markup.ValidName(topic) {
		panic("templatic: region topic must match [A-Za-z0-9_.-]+: " + topic)
	}
	cfg := buildConfig(opts)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := fmt.Fprintf(w, `<div data-tc-region=%q`, topic); err != nil {
			return err
		}
		if err := cfg.writeExtras(w); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `>`); err != nil {
			return err
		}
		if comp != nil {
			if err := comp.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
}
