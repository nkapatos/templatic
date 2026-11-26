// Package sri records a Subresource Integrity hash for every script and stylesheet the bundle writes.
//
// The hashes are sha384 values in the manifest's Integrity map, keyed by the
// output path relative to the dist directory. The templatic package adds
// them as integrity attributes on the stylesheet and modulepreload links it
// writes for files the manifest lists. [Integrity] computes one value for any bytes.
//
// Usage in an app build program (see [github.com/nkapatos/templatic/cli]):
//
//	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr, cli.With(sri.New())))
//
// Or with the generate package directly:
//
//	generate.Run(generate.Options{Plugins: []generate.Plugin{sri.New()}})
//
// The plugin hooks [generate.Builder.OnManifest].
package sri

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nkapatos/templatic/generate"
)

type plugin struct{}

// New returns the plugin.
func New() generate.Plugin { return plugin{} }

func (plugin) Name() string { return "sri" }

func (plugin) Setup(b *generate.Builder) error {
	b.OnManifest(func(m *generate.Manifest) error {
		for _, out := range m.Outputs {
			if !strings.HasSuffix(out.Path, ".js") && !strings.HasSuffix(out.Path, ".css") {
				continue
			}
			file := filepath.Join(m.Root, filepath.FromSlash(out.Path))
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(m.Dist, file)
			if err != nil {
				return err
			}
			if m.Integrity == nil {
				m.Integrity = map[string]string{}
			}
			m.Integrity[filepath.ToSlash(rel)] = Integrity(data)
		}
		return nil
	})
	return nil
}

// Integrity returns the SRI value of data: "sha384-{base64}".
func Integrity(data []byte) string {
	sum := sha512.Sum384(data)
	return fmt.Sprintf("sha384-%s", base64.StdEncoding.EncodeToString(sum[:]))
}
