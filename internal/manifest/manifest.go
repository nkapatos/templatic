// Package manifest reads the asset manifest (tc.json) that generate writes into dist.
// Manifest and Module are the data; Source hands out the current one, reloading it in dev.
// The root templatic package re-exports the types and pagestate carries the Source per request.
package manifest

import (
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nkapatos/templatic/internal/buildfiles"
)

// Manifest is the generated asset manifest (dist/tc.json).
type Manifest struct {
	// Version is the schema version; Load accepts only 2.
	Version int `json:"version"`
	// Mode is the build profile, "prod" or "dev".
	Mode string `json:"mode,omitempty"`
	// Base is the URL base the bundle was built to be served under.
	Base string `json:"base,omitempty"`
	// Modules maps an entry name to its output, css and static chunk imports.
	Modules map[string]Module `json:"modules,omitempty"`
	// Assets maps a root-relative source path to its dist-relative hashed output.
	Assets map[string]string `json:"assets,omitempty"`
	// Integrity maps a dist-relative output path to its SRI value.
	Integrity map[string]string `json:"integrity,omitempty"`
	// Binds maps an island tag to its reactive prop bindings.
	Binds map[string][]struct {
		Topic string `json:"topic"`
		Field string `json:"field"`
	} `json:"binds,omitempty"`
}

// Dev reports a manifest built in dev mode.
func (m *Manifest) Dev() bool { return m.Mode == "dev" }

// Module is one entry's row; paths are dist-relative.
type Module struct {
	File    string   `json:"file,omitempty"`
	CSS     []string `json:"css,omitempty"`
	Imports []string `json:"imports,omitempty"`
}

// Load reads the generated manifest in distDir; it refuses any version but 2.
func Load(distDir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(distDir, buildfiles.Manifest))
	if err != nil {
		return nil, fmt.Errorf("templatic: reading manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("templatic: parsing manifest: %w", err)
	}
	if m.Version != 2 {
		return nil, fmt.Errorf("templatic: manifest version %d is not 2: run templatic generate", m.Version)
	}
	return &m, nil
}

// Asset resolves p to its hashed URL; an unknown path panics in a dev manifest and
// logs and returns "" in prod.
func (m *Manifest) Asset(p string) string {
	if url, ok := m.AssetURL(p); ok {
		return url
	}
	msg := fmt.Sprintf("templatic: unknown asset %q (known: %s)", p, m.knownAssets())
	if m.Dev() {
		panic(msg)
	}
	slog.Error(msg)
	return ""
}

// AssetURL resolves p without Asset's panic and log policy.
func (m *Manifest) AssetURL(p string) (string, bool) {
	p = path.Clean(strings.TrimPrefix(strings.TrimPrefix(p, "/"), "./"))
	rel, ok := m.Assets[p]
	if !ok {
		return "", false
	}
	return strings.TrimSuffix(m.Base, "/") + "/" + rel, true
}

func (m *Manifest) knownAssets() string {
	if len(m.Assets) == 0 {
		return "none; the manifest has no assets map"
	}
	return strings.Join(slices.Sorted(maps.Keys(m.Assets)), ", ")
}
