package templatic

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/nkapatos/templatic/internal/manifest"
	"github.com/nkapatos/templatic/internal/pagestate"
)

// Manifest is the generated asset manifest (dist/tc.json).
type Manifest = manifest.Manifest

// ManifestModule is one entry in a Manifest, with paths relative to dist.
type ManifestModule = manifest.Module

// ManifestSource supplies the current manifest to layouts; in dev it follows
// rebuilds.
type ManifestSource = manifest.Source

// LoadManifest reads the generated manifest in distDir once.
func LoadManifest(distDir string) (*Manifest, error) { return manifest.Load(distDir) }

// ManifestFor loads the manifest in distDir; mode "dev" follows rebuilds and
// tolerates a missing manifest, any other mode reads it once.
func ManifestFor(mode, distDir string) (*ManifestSource, error) {
	return manifest.For(mode, distDir)
}

// DevManifest is ManifestFor("dev", distDir).
func DevManifest(distDir string) (*ManifestSource, error) { return manifest.For("dev", distDir) }

var defaultManifest atomic.Pointer[Manifest]

// UseManifest sets the manifest Asset resolves against outside a request.
func UseManifest(m *Manifest) { defaultManifest.Store(m) }

// Asset returns the content-hashed URL of the bundled asset at source path p, from the request's manifest (App or
// WithAssets put it in ctx, and it follows dev rebuilds), else the one UseManifest set. It panics on an unknown
// path in dev or when there is no manifest; in prod an unknown path logs an error and returns "".
func Asset(ctx context.Context, p string) string {
	m := pagestate.Manifest(ctx)
	if m == nil {
		m = defaultManifest.Load()
	}
	if m == nil {
		panic("templatic: Asset(" + fmt.Sprintf("%q", p) + ") has no manifest: serve the page through App or WithAssets, or call UseManifest")
	}
	return m.Asset(p)
}
