package templatic

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"strings"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/pagestate"
)

// BootOptions configures Boot.
type BootOptions struct {
	// Live is the mount base of the world, e.g. "/live".
	Live string
	// Assets is the URL base of the generated bundles, e.g. "/_templatic".
	Assets string
	// Manifest, when set, is inlined so the runtime skips the tc.json fetch.
	Manifest *Manifest
	// Prefetch enables Speculation Rules prefetching of same-origin links.
	Prefetch bool
	// Token is the page identity from world.PageToken; empty under WithPageTokens.
	Token string
	// Modules lists client page modules to modulepreload.
	Modules []string
}

// Boot emits the live config and the boot module script; place it once, at
// the end of the layout body.
func Boot(o BootOptions) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		base := strings.TrimSuffix(o.Live, "/")
		o := o // per render: ctx fills must not stick to the component
		token := o.Token
		if token == "" {
			token = PageToken(ctx) // templatic.WithPageTokens
		}
		if o.Manifest == nil {
			o.Manifest = pagestate.Manifest(ctx) // templatic.WithAssets
		}
		assets := strings.TrimSuffix(o.Assets, "/")
		cfg := struct {
			Stream   string           `json:"stream"`
			Commands string           `json:"commands"`
			Assets   string           `json:"assets"`
			Manifest *runtimeManifest `json:"manifest,omitempty"`
			Token    string           `json:"token,omitempty"`
			Modules  []string         `json:"modules,omitempty"`
		}{
			Stream:   base + "/stream",
			Commands: base + "/commands",
			Assets:   assets,
			Manifest: newRuntimeManifest(o.Manifest),
			Token:    token,
			Modules:  o.Modules,
		}
		// Preload hints precede the config so their fetches start while boot.js
		// downloads.
		if o.Manifest != nil {
			ctx = pagestate.WithState(ctx)
			var sb strings.Builder
			for _, tag := range pagestate.RenderedIslands(ctx) {
				writeIslandLinks(ctx, &sb, o.Manifest, assets, o.Manifest.Modules["islands/"+tag])
			}
			for _, name := range o.Modules {
				mod := o.Manifest.Modules["modules/"+name]
				writeLinks(ctx, &sb, o.Manifest, assets, "modulepreload", append([]string{mod.File}, mod.Imports...)...)
			}
			if _, err := io.WriteString(w, sb.String()); err != nil {
				return err
			}
		}
		if o.Prefetch {
			if _, err := io.WriteString(w, `<script type="speculationrules"`+nonceAttr(ctx)+`>{"prefetch":[{"where":{"href_matches":"/*"},"eagerness":"moderate"}]}</script>`); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, `<script type="application/tc-config">`); err != nil {
			return err
		}
		if err := json.MarshalWrite(w, cfg, jsontext.EscapeForHTML(true)); err != nil {
			return err
		}
		// A manifest names the content-hashed boot bundle; otherwise the boot.js
		// shim loads it.
		bootFile := "boot.js"
		if o.Manifest != nil && o.Manifest.Modules["boot"].File != "" {
			bootFile = o.Manifest.Modules["boot"].File
		}
		if _, err := fmt.Fprintf(w, `</script><script type="module" src=%q%s async></script>`,
			templ.EscapeString(assets+"/"+bootFile), nonceAttr(ctx)); err != nil {
			return err
		}
		return nil
	})
}

// runtimeManifest is the part of the manifest the client runtime reads; assets
// and integrity stay server-side so they are not inlined into every page.
type runtimeManifest struct {
	Modules map[string]ManifestModule `json:"modules,omitempty"`
	Binds   map[string][]struct {
		Topic string `json:"topic"`
		Field string `json:"field"`
	} `json:"binds,omitempty"`
}

func newRuntimeManifest(m *Manifest) *runtimeManifest {
	if m == nil {
		return nil
	}
	return &runtimeManifest{Modules: m.Modules, Binds: m.Binds}
}
