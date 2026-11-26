// Package generate runs the templatic compiler pass: Go glue, island TypeScript, runtime copy and esbuild bundle.
package generate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Options configure one generate run.
type Options struct {
	// Root is the module root containing the .templ sources and go.mod; default ".".
	Root string
	// ClientDir is the root-relative tree of the app's own client code; default "client".
	ClientDir string
	// DistDir is the root-relative bundle output directory; default ".templatic/dist".
	// It must lie inside Root and must not overlap GenDir, ClientDir or any source.
	DistDir string
	// AssetsBase is the URL base DistDir is served under; default "/_templatic/".
	AssetsBase string
	// SkipBundle skips the esbuild step; islands and the runtime are still written.
	SkipBundle bool

	// Mode selects the build profile; the zero value is ModeProd.
	Mode Mode

	// Alias maps import paths to replacements, applied before OnBuildOptions hooks.
	Alias map[string]string

	// Loader maps file extensions to esbuild loaders, applied before OnBuildOptions hooks.
	Loader map[string]api.Loader

	// Defines are esbuild compile-time constant replacements.
	Defines map[string]string

	// Plugins are templatic plugins, run in registration order.
	Plugins []Plugin

	// ESBuildPlugins are native esbuild Go plugins, run in-process.
	ESBuildPlugins []api.Plugin

	// Build mutates the esbuild options immediately before the build, after every other hook.
	Build func(*api.BuildOptions)
}

// Mode is the build profile.
type Mode string

const (
	// ModeProd is the default: minified, no sourcemaps.
	ModeProd Mode = "prod"
	// ModeDev emits unminified bundles with linked sourcemaps and the dev defines.
	ModeDev Mode = "dev"
)

// Island is one `//templatic:island` component discovered by the pass.
type Island struct {
	// Tag is the custom element name, lowercase kebab.
	Tag string
	// PropsType is the Go struct name feeding the island's typed props.
	PropsType string
	// Variant is "" (hooks) or "lit" (LitElement subclass).
	Variant string
	// Binds are the island's reactive prop bindings (lit only).
	Binds []Bind
	// Component is the templ component name carrying the pragma.
	Component string
	// File is the .templ source path relative to Root.
	File string
	// FileDir is the directory of File, for package-local type walks.
	FileDir string
	// Script and CSS are the extracted single-file-component sources.
	Script string
	CSS    string
}

// Summary reports what one run did.
type Summary struct {
	// Emitted lists the Go files written, as root-relative paths.
	Emitted []string
	// Islands lists discovered islands.
	Islands []Island
	// RuntimeFiles lists runtime sources written into GenDir/runtime.
	RuntimeFiles []string
	// Bundle lists the bundle outputs, root-relative, in stable order.
	Bundle []string
	// Manifest is the tc.json location, set when the bundle ran.
	Manifest string
	// Watch lists the extra root-relative globs plugins asked a dev loop to watch.
	Watch []string
}

// TemplVersion is the templ release this pass renders Go glue with.
const TemplVersion = templVersion

const (
	templVersion = "v0.3.1118"
	pragmaPrefix = "//templatic:"
)

// Run executes the full pass under opts.Root; it fails on the first error and leaves written outputs in place.
func Run(opts Options) (*Summary, error) {
	if err := opts.Mode.validate(); err != nil {
		return nil, err
	}
	opts.applyDefaults()
	b, err := newBuilder(opts.Plugins)
	if err != nil {
		return nil, err
	}
	if err := runHooks(b.config, &opts, func(f func(*Options) error, o *Options) error { return f(o) }); err != nil {
		return nil, err
	}
	if err := opts.Mode.validate(); err != nil {
		return nil, err
	}
	opts.applyDefaults()
	if err := opts.validatePaths(); err != nil {
		return nil, err
	}
	if err := ensureGenDir(opts); err != nil {
		return nil, err
	}
	if err := ensureDist(filepath.Join(opts.Root, opts.DistDir)); err != nil {
		return nil, err
	}

	s := &Summary{Watch: b.watch}

	files, err := findTemplFiles(opts.Root)
	if err != nil {
		return nil, err
	}
	if err := removeOrphans(opts.Root); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("generate: no .templ files under %s", opts.Root)
	}

	var unresolved []unresolvedQuals
	for _, rel := range files {
		islands, err := renderFile(opts, rel, s, &unresolved)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		s.Islands = append(s.Islands, islands...)
	}
	if err := checkUnresolved(unresolved); err != nil {
		return nil, err
	}
	slices.Sort(s.Emitted)

	prog, err := loadProgram(opts.Root)
	if err != nil {
		return nil, err
	}
	if err := checkBinds(prog, s.Islands); err != nil {
		return nil, err
	}
	if err := writeCommandsTS(opts, prog); err != nil {
		return nil, err
	}
	for _, isl := range s.Islands {
		if err := writeIslandTS(opts, prog, isl); err != nil {
			return nil, fmt.Errorf("island %s: %w", isl.Tag, err)
		}
	}

	if err := copyRuntime(opts, s); err != nil {
		return nil, err
	}

	if err := runHooks(b.scan, s, func(f func(*Summary) error, s *Summary) error { return f(s) }); err != nil {
		return nil, err
	}

	if opts.SkipBundle {
		return s, nil
	}
	if err := bundle(opts, s, b); err != nil {
		return nil, err
	}
	return s, nil
}

func (m Mode) validate() error {
	switch m {
	case "", ModeProd, ModeDev:
		return nil
	}
	return fmt.Errorf("generate: unknown mode %q (want %q or %q)", m, ModeProd, ModeDev)
}

func (opts *Options) applyDefaults() {
	if opts.Root == "" {
		opts.Root = "."
	}
	if opts.ClientDir == "" {
		opts.ClientDir = "client"
	}
	if opts.DistDir == "" {
		opts.DistDir = defaultDistDir
	}
	opts.ClientDir = filepath.Clean(opts.ClientDir)
	opts.DistDir = filepath.Clean(opts.DistDir)
	if opts.AssetsBase == "" {
		opts.AssetsBase = DefaultAssetsBase
	}
}

const templGoHeader = "// Code generated by templ - DO NOT EDIT.\n"

// removeOrphans deletes x_templ.go and x_tc.go whose x.templ is gone; a hand-written file with the same suffix is left alone.
func removeOrphans(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		var src, header string
		switch {
		case strings.HasSuffix(p, "_templ.go"):
			src, header = strings.TrimSuffix(p, "_templ.go")+".templ", templGoHeader
		case strings.HasSuffix(p, "_tc.go"):
			src, header = strings.TrimSuffix(p, "_tc.go")+".templ", genGoHeader
		default:
			return nil
		}
		if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if b, err := os.ReadFile(p); err != nil || !bytes.HasPrefix(b, []byte(header)) {
			return nil
		}
		return os.Remove(p)
	})
}

func skipDir(name string) bool {
	switch name {
	case "node_modules", "vendor", ".git", "dist", "tc_gen", GenDir:
		return true
	}
	return false
}

func findTemplFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".templ") {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	return files, nil
}
