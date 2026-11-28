package templatic

import (
	"context"
	"io"
	"strings"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/markup"
	"github.com/nkapatos/templatic/internal/pagestate"
)

// HeadOptions configures Head.
type HeadOptions struct {
	// Assets is the URL base of the bundle; it must match BootOptions.Assets.
	Assets string
	// Manifest overrides the manifest from the request context.
	Manifest *Manifest
	// Styles names manifest entries whose CSS is linked.
	Styles []string
	// Modules names manifest entries whose JS is modulepreloaded with its imports.
	Modules []string
	// Preload lists island tags whose CSS is linked and module modulepreloaded.
	Preload []string
}

// Head writes the stylesheet and modulepreload links for <head>,
// de-duplicated, with integrity hashes when the manifest has them.
func Head(o HeadOptions) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		m := o.Manifest
		if m == nil {
			m = pagestate.Manifest(ctx)
		}
		if m == nil {
			return nil
		}
		ctx = pagestate.WithState(ctx)
		base := strings.TrimSuffix(o.Assets, "/")
		var sb strings.Builder
		preload := func(mod ManifestModule) {
			writeLinks(ctx, &sb, m, base, "modulepreload", append([]string{mod.File}, mod.Imports...)...)
		}

		styles, modules := o.Styles, o.Modules
		if _, ok := m.Modules["app"]; ok {
			styles = append([]string{"app"}, styles...)
			modules = append([]string{"app"}, modules...)
		}
		for _, name := range styles {
			writeLinks(ctx, &sb, m, base, "stylesheet", m.Modules[name].CSS...)
		}
		for _, tag := range o.Preload {
			writeLinks(ctx, &sb, m, base, "stylesheet", m.Modules["islands/"+tag].CSS...)
		}
		preload(m.Modules["boot"])
		for _, name := range modules {
			preload(m.Modules[name])
		}
		for _, tag := range o.Preload {
			preload(m.Modules["islands/"+tag])
		}
		_, err := io.WriteString(w, sb.String())
		return err
	})
}

// writeLinks is the one link writer: it writes a <link rel> per file not yet
// written for this request, with integrity when the manifest has it, and
// records each URL in pagestate so a later writer skips it.
func writeLinks(ctx context.Context, sb *strings.Builder, m *Manifest, base, rel string, files ...string) {
	for _, file := range files {
		if file == "" || !pagestate.NoteLink(ctx, base+"/"+file) {
			continue
		}
		sb.WriteString(`<link rel="` + rel + `"`)
		if rel == "preload" { // only css is preloaded
			sb.WriteString(` as="style"`)
		}
		sb.WriteString(` href="` + markup.EscapeAttr(base+"/"+file) + `"`)
		if rel == "modulepreload" {
			sb.WriteString(nonceAttr(ctx))
		}
		if sum := m.Integrity[file]; sum != "" {
			sb.WriteString(` integrity="` + markup.EscapeAttr(sum) + `"`)
		}
		sb.WriteString(">")
	}
}

// writeIslandLinks is Boot's per-island call: the module and its imports as
// modulepreload, its css as preload.
func writeIslandLinks(ctx context.Context, sb *strings.Builder, m *Manifest, base string, mod ManifestModule) {
	writeLinks(ctx, sb, m, base, "modulepreload", append([]string{mod.File}, mod.Imports...)...)
	writeLinks(ctx, sb, m, base, "preload", mod.CSS...)
}

// nonceAttr is ` nonce="…"` for the CSP nonce templ.WithNonce set, or "" without one.
func nonceAttr(ctx context.Context) string {
	n := templ.GetNonce(ctx)
	if n == "" {
		return ""
	}
	return ` nonce="` + markup.EscapeAttr(n) + `"`
}
