package generate

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Plugin extends the generate pass; a nil plugin is skipped, and a Setup or hook error aborts the run.
type Plugin interface {
	Name() string
	Setup(b *Builder) error
}

// Builder is the registration surface handed to Plugin.Setup; hook calls accumulate.
type Builder struct {
	cur string

	config    []hook[func(*Options) error]
	scan      []hook[func(*Summary) error]
	entries   []hook[func(*Entries) error]
	esbuild   []api.Plugin
	buildOpts []hook[func(*api.BuildOptions) error]
	manifest  []hook[func(*Manifest) error]
	done      []hook[func(*Result) error]
	watch     []string
}

type hook[F any] struct {
	plugin string
	fn     F
}

// OnConfig adjusts the options before anything runs; defaults are re-applied after.
func (b *Builder) OnConfig(fn func(*Options) error) {
	b.config = append(b.config, hook[func(*Options) error]{b.cur, fn})
}

// OnScan runs after templ parsing, SFC takeover and island and runtime writing.
func (b *Builder) OnScan(fn func(*Summary) error) {
	b.scan = append(b.scan, hook[func(*Summary) error]{b.cur, fn})
}

// OnEntries adds or alters esbuild entries before the build is configured.
func (b *Builder) OnEntries(fn func(*Entries) error) {
	b.entries = append(b.entries, hook[func(*Entries) error]{b.cur, fn})
}

// ESBuild adds a native esbuild plugin, run after Options.ESBuildPlugins.
func (b *Builder) ESBuild(p api.Plugin) { b.esbuild = append(b.esbuild, p) }

// OnBuildOptions adjusts the esbuild options last, before Options.Build.
func (b *Builder) OnBuildOptions(fn func(*api.BuildOptions) error) {
	b.buildOpts = append(b.buildOpts, hook[func(*api.BuildOptions) error]{b.cur, fn})
}

// OnManifest edits the manifest after hashing, before tc.json is written.
func (b *Builder) OnManifest(fn func(*Manifest) error) {
	b.manifest = append(b.manifest, hook[func(*Manifest) error]{b.cur, fn})
}

// OnDone runs last, after tc.json is written and dist is pruned.
func (b *Builder) OnDone(fn func(*Result) error) {
	b.done = append(b.done, hook[func(*Result) error]{b.cur, fn})
}

// Watch records extra root-relative globs for the dev loop to watch.
func (b *Builder) Watch(globs ...string) { b.watch = append(b.watch, globs...) }

// ESBuild adapts a native esbuild plugin into a Plugin.
func ESBuild(p api.Plugin) Plugin { return esbuildPlugin{p} }

type esbuildPlugin struct{ p api.Plugin }

func (e esbuildPlugin) Name() string           { return e.p.Name }
func (e esbuildPlugin) Setup(b *Builder) error { b.ESBuild(e.p); return nil }

func newBuilder(plugins []Plugin) (*Builder, error) {
	b := &Builder{}
	for _, p := range plugins {
		if isNil(p) {
			continue
		}
		b.cur = p.Name()
		if err := p.Setup(b); err != nil {
			return nil, attribute(b.cur, err)
		}
	}
	b.cur = ""
	return b, nil
}

// isNil reports nil plugins, including a typed nil that would panic in Name.
func isNil(p Plugin) bool {
	if p == nil {
		return true
	}
	v := reflect.ValueOf(p)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Slice, reflect.Chan, reflect.Interface:
		return v.IsNil()
	}
	return false
}

func attribute(plugin string, err error) error {
	return fmt.Errorf("plugin %s: %w", plugin, err)
}

func runHooks[F any, A any](hooks []hook[F], arg A, call func(F, A) error) error {
	for _, h := range hooks {
		if err := call(h.fn, arg); err != nil {
			return attribute(h.plugin, err)
		}
	}
	return nil
}

// EntryKind says where an entry came from.
type EntryKind string

const (
	// EntryBoot is the runtime bootstrap.
	EntryBoot EntryKind = "boot"
	// EntryIsland is an island define module.
	EntryIsland EntryKind = "island"
	// EntryExtra is an entry a plugin added.
	EntryExtra EntryKind = "extra"
)

// Entry is one esbuild entry point.
type Entry struct {
	// Name is the extensionless output path under dist.
	Name string
	// Input is the absolute source path.
	Input string
	Kind  EntryKind
}

// Entries is the entry table OnEntries hooks may extend.
type Entries struct {
	// Root is the absolute module root.
	Root string
	// List is every entry, boot first, islands in discovery order.
	List []Entry
}

// Add appends an extra entry; input is absolute or root-relative.
func (e *Entries) Add(name, input string) {
	if !filepath.IsAbs(input) {
		input = filepath.Join(e.Root, filepath.FromSlash(input))
	}
	e.List = append(e.List, Entry{Name: name, Input: input, Kind: EntryExtra})
}

func (e *Entries) validate() error {
	seen := map[string]bool{}
	for _, en := range e.List {
		n := en.Name
		if n == "" || strings.Contains(n, "..") || strings.HasPrefix(n, "/") || strings.Contains(n, "\\") {
			return fmt.Errorf("generate: invalid entry name %q", n)
		}
		if seen[n] {
			return fmt.Errorf("generate: duplicate entry name %q", n)
		}
		seen[n] = true
		if en.Kind == EntryExtra && (n == "boot" || n == "tc" || strings.HasPrefix(n, "islands/") || strings.HasPrefix(n, "chunks/") || strings.HasPrefix(n, "assets/")) {
			return fmt.Errorf("generate: entry name %q is reserved", n)
		}
	}
	return nil
}

// ManifestBind is one reactive prop binding in tc.json's binds map.
type ManifestBind struct {
	Topic string `json:"topic"`
	Field string `json:"field"`
}

// Manifest is the content of tc.json while it is built.
type Manifest struct {
	// Binds maps tag to its reactive prop bindings.
	Binds map[string][]ManifestBind `json:"binds,omitempty"`
	// Meta is free-form plugin metadata, namespaced by plugin name.
	Meta map[string]any `json:"meta,omitempty"`

	// Version is the manifest schema version, 2.
	Version int `json:"version,omitzero"`
	// Mode is the build profile, "prod" or "dev".
	Mode string `json:"mode,omitempty"`
	// Base is the URL base dist is served under.
	Base string `json:"base,omitempty"`
	// Modules maps each entry's name to its output, css and static chunk imports.
	Modules map[string]ManifestModule `json:"modules,omitempty"`
	// Assets maps a static asset's source path to its hashed output.
	Assets map[string]string `json:"assets,omitempty"`
	// Integrity maps an output path to its Subresource Integrity value.
	Integrity map[string]string `json:"integrity,omitempty"`

	// Root is the absolute module root, Dist the dist directory, Outputs the hashed outputs.
	Root    string   `json:"-"`
	Dist    string   `json:"-"`
	Outputs []Output `json:"-"`
}

// ManifestModule is one entry's row in the modules map; paths are dist-relative.
type ManifestModule struct {
	// File is the entry's hashed JS output; "" for a CSS-only entry.
	File string `json:"file,omitempty"`
	// CSS lists the stylesheets the entry produced.
	CSS []string `json:"css,omitempty"`
	// Imports are the entry's static chunk imports, in first-reached order.
	Imports []string `json:"imports,omitempty"`
}

// Import is one import edge recorded by esbuild.
type Import struct {
	// Path is the dist output path (root-relative, slash) imported.
	Path string
	// Kind is esbuild's import kind, "import-statement" or "dynamic-import".
	Kind string
}

// Output is one file the bundle wrote.
type Output struct {
	// Path is the final root-relative slash path (after content hashing).
	Path string
	// Entry is the root-relative source entry point; "" for chunks and assets.
	Entry string
	Bytes int
	// Imports are this output's import edges, with final paths.
	Imports []Import
	// CSS is the stylesheet bundled for this entry, "" if none.
	CSS string
	// Inputs are the sorted root-relative source files that fed this output.
	Inputs []string
}

// Result is what OnDone hooks receive: everything one bundle produced.
type Result struct {
	// Root and Dist are absolute.
	Root string
	Dist string
	// Summary is the run's summary (islands, outputs, manifest path).
	Summary *Summary
	// Manifest is the manifest as written.
	Manifest *Manifest
	// Metafile is esbuild's raw metafile JSON, with pre-hashing paths.
	Metafile string
	// Outputs are the bundle's files with final paths.
	Outputs []Output
}
