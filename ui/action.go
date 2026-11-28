package ui

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/form"
	"github.com/nkapatos/templatic/internal/markup"
	"github.com/nkapatos/templatic/live"
)

// Action renders a button that raises a world command, sending args as JSON.
//
// Deprecated: use Button.
func Action(name string, args any, opts ...Option) templ.Component {
	if !markup.ValidName(name) {
		panic("templatic: action name must match [A-Za-z0-9_.-]+: " + name)
	}
	cfg := buildConfig(opts)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := fmt.Fprintf(w, `<button type="button" data-tc-action=%q`, name); err != nil {
			return err
		}
		if err := cfg.writeExtras(w); err != nil {
			return err
		}
		if args != nil {
			enc, err := json.Marshal(args)
			if err != nil {
				return err
			}
			if err := markup.WriteStrings(w, ` data-tc-args="`, markup.EscapeAttr(string(enc)), `"`); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, `>`); err != nil {
			return err
		}
		if children := templ.GetChildren(ctx); children != nil {
			if err := children.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</button>`)
		return err
	})
}

// LinkOption configures a link; it is an Option.
type LinkOption = Option

// Swaps declares the region the trigger's command swaps; where the page lacks
// it, the browser follows the href instead.
func Swaps(topic string) LinkOption {
	return func(c *elemConfig) { c.swaps = topic }
}

// ActionLink renders an anchor that raises a world command and falls back to
// href without JavaScript.
//
// Deprecated: use Link.
func ActionLink(name string, args any, href string, opts ...LinkOption) templ.Component {
	if !markup.ValidName(name) {
		panic("templatic: action name must match [A-Za-z0-9_.-]+: " + name)
	}
	if href == "" {
		panic("templatic: action link needs a no-JS fallback href")
	}
	// href gets templ's scheme filter plus HTML escaping, not %q, whose \" escapes
	// decode back to a quote and allow attribute injection.
	safeHref := templ.EscapeString(string(templ.URL(href)))
	cfg := buildConfig(opts)
	if cfg.swaps != "" && !markup.ValidName(cfg.swaps) {
		panic("templatic: swap target must match [A-Za-z0-9_.-]+: " + cfg.swaps)
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := fmt.Fprintf(w, `<a href=%q data-tc-action=%q`, safeHref, name); err != nil {
			return err
		}
		if cfg.swaps != "" {
			if _, err := fmt.Fprintf(w, ` data-tc-swaps=%q`, cfg.swaps); err != nil {
				return err
			}
		}
		if err := cfg.writeExtras(w); err != nil {
			return err
		}
		if args != nil {
			enc, err := json.Marshal(args)
			if err != nil {
				return err
			}
			if err := markup.WriteStrings(w, ` data-tc-args="`, markup.EscapeAttr(string(enc)), `"`); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, `>`); err != nil {
			return err
		}
		if children := templ.GetChildren(ctx); children != nil {
			if err := children.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</a>`)
		return err
	})
}

// PostTo makes a Button a one-button form that POSTs its input to action, where cmd.Form serves it with and
// without JavaScript.
func PostTo(action string) Option {
	return func(c *elemConfig) { c.postTo = action }
}

// Button renders a trigger button that raises cmd with in; with PostTo it is a one-button form that POSTs in to
// the action URL, with and without JavaScript. It panics when PostTo is given and In has a field no form can fill.
func Button[In any](cmd *live.Command[In], in In, opts ...Option) templ.Component {
	cfg := buildConfig(opts)
	if cfg.postTo == "" {
		return Action(cmd.Name(), in, opts...)
	}
	fields, err := form.Encode(in)
	if err != nil {
		panic("templatic: ui.Button(" + cmd.Name() + ") with PostTo: " + err.Error())
	}
	names := slices.Sorted(maps.Keys(fields))
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		label := templ.GetChildren(ctx) // the caller's content, before the form replaces the children
		button := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			for _, name := range names {
				for _, v := range fields[name] {
					if err := markup.WriteStrings(w, `<input type="hidden" name="`, markup.EscapeAttr(name), `" value="`, markup.EscapeAttr(v), `">`); err != nil {
						return err
					}
				}
			}
			// the options style the button; the form is bare
			if _, err := io.WriteString(w, `<button type="submit"`); err != nil {
				return err
			}
			if err := cfg.writeExtras(w); err != nil {
				return err
			}
			if _, err := io.WriteString(w, `>`); err != nil {
				return err
			}
			if label != nil {
				if err := label.Render(templ.ClearChildren(ctx), w); err != nil {
					return err
				}
			}
			_, err := io.WriteString(w, `</button>`)
			return err
		})
		return Form(cmd.Name(), cfg.postTo).Render(templ.WithChildren(ctx, button), w)
	})
}

// Link renders an anchor that raises cmd with in and follows href without
// JavaScript, applying the region cmd declares unless Swaps overrides it.
func Link[In any](cmd *live.Command[In], in In, href string, opts ...Option) templ.Component {
	if t := cmd.SwapTopic(); t != "" {
		opts = append([]Option{Swaps(t)}, opts...)
	}
	return ActionLink(cmd.Name(), in, href, opts...)
}
