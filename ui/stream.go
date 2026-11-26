package ui

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/markup"
)

// Stream renders the container that live appends to topic target; new items
// are prepended unless Tail is given.
func Stream(topic string, comp templ.Component, opts ...StreamOption) templ.Component {
	if !markup.ValidName(topic) {
		panic("templatic: stream topic must match [A-Za-z0-9_.-]+: " + topic)
	}
	cfg := buildConfig(opts)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		dir := ""
		if cfg.tail {
			dir = ` data-tc-dir="tail"`
		}
		if _, err := fmt.Fprintf(w, `<div data-tc-stream=%q%s`, topic, dir); err != nil {
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

// StreamOption configures a Stream container.
type StreamOption = Option

// Tail appends live arrivals below existing items instead of on top.
func Tail() StreamOption {
	return func(c *elemConfig) { c.tail = true }
}

// StreamItem renders one keyed stream item in the wrapper World.Append emits;
// a key outside [A-Za-z0-9_.-]+ fails the render.
func StreamItem(key string, comp templ.Component, opts ...Option) templ.Component {
	cfg := buildConfig(opts)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if !markup.ValidName(key) {
			return fmt.Errorf("templatic: stream key must match [A-Za-z0-9_.-]+: %q", key)
		}
		if _, err := fmt.Fprintf(w, `<div data-tc-item=%q`, key); err != nil {
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
