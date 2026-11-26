package generate

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/nkapatos/templatic/internal/buildfiles"
)

// GenDir is the one directory generate owns; it is gitignored and safe to delete.
const GenDir = ".templatic"

// MarkerFile is the file inside GenDir that marks it as made by templatic.
const MarkerFile = ".templatic"

const (
	generationsFile = buildfiles.Generations
	defaultDistDir  = DefaultDistDir
	// DefaultDistDir is the dist dir inside GenDir.
	DefaultDistDir = GenDir + "/dist"
	// DefaultAssetsBase is the URL base the default dist dir is mounted under.
	DefaultAssetsBase = "/_templatic/"
)

func (opts Options) genDir() string { return filepath.Join(opts.Root, GenDir) }

func within(dir, p string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

func (opts Options) validatePaths() error {
	client, dist := opts.ClientDir, opts.DistDir
	if !filepath.IsLocal(client) {
		return fmt.Errorf("generate: ClientDir %q must be a relative path inside the module root", client)
	}
	if !filepath.IsLocal(dist) {
		return fmt.Errorf("generate: DistDir %q must be a relative path inside the module root", dist)
	}
	d := filepath.ToSlash(dist)
	c := filepath.ToSlash(client)
	switch {
	case d == ".":
		return fmt.Errorf("generate: DistDir must not be the module root")
	case d == GenDir:
		return fmt.Errorf("generate: DistDir must not be %s itself: use %s or a directory outside it", GenDir, defaultDistDir)
	case within(GenDir, d) && !within(defaultDistDir, d):
		return fmt.Errorf("generate: DistDir %s is inside %s, which holds generated sources: use %s", d, GenDir, defaultDistDir)
	case within(d, c):
		return fmt.Errorf("generate: DistDir %s equals or contains ClientDir %s", d, c)
	case within(d, GenDir):
		return fmt.Errorf("generate: DistDir %s contains %s", d, GenDir)
	}
	abs := filepath.Join(opts.Root, dist)
	if err := rejectSources(abs); err != nil {
		return fmt.Errorf("generate: DistDir %s: %w", d, err)
	}
	empty, err := dirEmpty(abs)
	if err != nil {
		return err
	}
	if !empty {
		if _, err := os.Stat(filepath.Join(abs, generationsFile)); err != nil {
			return fmt.Errorf("generate: %s is not empty and was not created by templatic: empty it or choose another DistDir", d)
		}
	}
	return nil
}

func rejectSources(dir string) error {
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !e.IsDir() && (strings.HasSuffix(p, ".templ") || strings.HasSuffix(p, ".go")) {
			rel, _ := filepath.Rel(dir, p)
			return fmt.Errorf("holds source file %s", filepath.ToSlash(rel))
		}
		return nil
	})
	return err
}

func dirEmpty(dir string) (bool, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("generate: %w", err)
	}
	return len(ents) == 0, nil
}

func templaticVersion() string {
	const mod = "github.com/nkapatos/templatic"
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Path == mod && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
		for _, d := range bi.Deps {
			if d.Path == mod {
				if d.Replace != nil || d.Version == "" || d.Version == "(devel)" {
					return "dev"
				}
				return d.Version
			}
		}
	}
	return "dev"
}

func ensureGenDir(opts Options) error {
	dir := opts.genDir()
	marker := filepath.Join(dir, MarkerFile)
	if _, err := os.Stat(marker); err != nil {
		empty, derr := dirEmpty(dir)
		if derr != nil {
			return derr
		}
		if !empty {
			return fmt.Errorf("generate: %s exists and was not created by templatic: remove it", GenDir)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body, _ := json.Marshal(struct {
		Version string `json:"version"`
	}{templaticVersion()})
	return writeAtomic(marker, append(body, '\n'))
}

// HasMarker reports whether root/GenDir carries the templatic marker.
func HasMarker(root string) bool {
	_, err := os.Stat(filepath.Join(root, GenDir, MarkerFile))
	return err == nil
}

type generations struct {
	Generations [][]string `json:"generations"`
}

func readGenerations(dist string) (generations, error) {
	var g generations
	data, err := os.ReadFile(filepath.Join(dist, generationsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return g, nil
	}
	if err != nil {
		return g, err
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return g, fmt.Errorf("generate: %s: %w", filepath.Join(dist, generationsFile), err)
	}
	return g, nil
}

func writeGenerations(dist string, g generations) error {
	if g.Generations == nil {
		g.Generations = [][]string{}
	}
	data, err := json.Marshal(g, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dist, generationsFile), append(data, '\n'))
}

func ensureDist(dist string) error {
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dist, generationsFile)); err == nil {
		return nil
	}
	return writeGenerations(dist, generations{})
}

// recordGeneration deletes a file only when an older generation recorded it; the previous
// generation stays so open pages still load their lazy chunks.
func recordGeneration(dist string, files []string) error {
	g, err := readGenerations(dist)
	if err != nil {
		return err
	}
	cur := append([]string(nil), files...)
	slices.Sort(cur)
	g.Generations = append(g.Generations, cur)
	if err := writeGenerations(dist, g); err != nil {
		return err
	}
	if len(g.Generations) <= 2 {
		return nil
	}
	n := len(g.Generations)
	keep := map[string]bool{}
	for _, gen := range g.Generations[n-2:] {
		for _, f := range gen {
			keep[f] = true
		}
	}
	for _, gen := range g.Generations[:n-2] {
		for _, f := range gen {
			if keep[f] {
				continue
			}
			keep[f] = true
			if !path.IsAbs(f) && filepath.IsLocal(filepath.FromSlash(f)) {
				if err := os.Remove(filepath.Join(dist, filepath.FromSlash(f))); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
		}
	}
	g.Generations = g.Generations[n-2:]
	return writeGenerations(dist, g)
}

// writeAtomic replaces path via rename, and leaves an identical file untouched so the dev watcher sees no no-op rewrites.
func writeAtomic(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
