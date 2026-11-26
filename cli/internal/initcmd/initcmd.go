// Package initcmd implements `templatic init`: it scaffolds a new app.
package initcmd

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"text/template"

	"github.com/nkapatos/templatic/generate"
	"github.com/nkapatos/templatic/internal/clientruntime"
	"github.com/urfave/cli/v3"
	"golang.org/x/mod/module"
)

//go:embed all:scaffold
var scaffold embed.FS

const templaticModule = "github.com/nkapatos/templatic"

// goVersion is templatic's minimum Go version (its go.mod directive): the floor of the scaffold's go line.
const goVersion = "1.27.4"

// replaceVersion is the placeholder require version of a -replace checkout.
const replaceVersion = "v0.0.0"

type scaffoldData struct {
	Module, Name          string
	Version, TemplVersion string
	GoVersion             string
	Replace               string
	ClientDeps            []npmDep
}

type npmDep struct{ Name, Version string }

// clientDeps pins the tested versions; generate accepts any version in the supported range.
func clientDeps() []npmDep {
	var deps []npmDep
	for name, v := range clientruntime.TestedVersions() {
		deps = append(deps, npmDep{name, v})
	}
	slices.SortFunc(deps, func(a, b npmDep) int { return strings.Compare(a.Name, b.Name) })
	return deps
}

// Command returns `templatic init`, which scaffolds a new app.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "scaffold a new app",
		ArgsUsage: "<dir>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "module", Usage: "Go module path (default: the directory name)"},
			&cli.StringFlag{Name: "version", Usage: "templatic version to require (default: this binary's)"},
			&cli.StringFlag{Name: "replace", Usage: "path to a local templatic checkout (adds a go.mod replace)"},
			&cli.BoolFlag{Name: "force", Usage: "write into a non-empty directory (existing files are kept)"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 1 {
				return cli.Exit("usage: templatic init [flags] <dir>", 2)
			}
			dir, stdout := cmd.Args().First(), cmd.Root().Writer
			data, err := newScaffoldData(dir, cmd.String("module"), cmd.String("version"), cmd.String("replace"))
			if err != nil {
				return err
			}
			written, err := writeScaffold(dir, data, cmd.Bool("force"))
			if err != nil {
				return err
			}
			for _, f := range written {
				fmt.Fprintf(stdout, "  %s\n", filepath.Join(dir, f))
			}
			fmt.Fprintf(stdout, `
templatic: %s is ready. Next:

  cd %s
  go mod tidy
  npm install
  go tool templatic generate
  go run .            # http://localhost:8080

`, data.Module, dir)
			return nil
		},
	}
}

func newScaffoldData(dir, modulePath, version, replace string) (scaffoldData, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return scaffoldData{}, err
	}
	name := filepath.Base(abs)
	if modulePath == "" {
		modulePath = name
	}
	if err := module.CheckImportPath(modulePath); err != nil {
		return scaffoldData{}, fmt.Errorf("init: %q is not a valid module path (--module): %w", modulePath, err)
	}
	if version == "" {
		if v, ok := binaryVersion(); ok {
			version = v
		} else if replace == "" {
			return scaffoldData{}, errors.New("init: this templatic binary has no release version (a dev build): pass --version vX.Y.Z or --replace <checkout>")
		} else {
			version = replaceVersion
		}
	}
	if replace != "" {
		r, err := filepath.Abs(replace)
		if err != nil {
			return scaffoldData{}, err
		}
		if _, err := os.Stat(filepath.Join(r, "go.mod")); err != nil {
			return scaffoldData{}, fmt.Errorf("init: --replace %s: no go.mod there", replace)
		}
		replace = filepath.ToSlash(r)
	}
	return scaffoldData{
		Module:       modulePath,
		Name:         path.Base(modulePath),
		Version:      version,
		TemplVersion: generate.TemplVersion,
		GoVersion:    goVersion,
		Replace:      replace,
		ClientDeps:   clientDeps(),
	}, nil
}

// binaryVersion is the release version of templatic this binary was built from; ok is false for a dev build.
func binaryVersion() (version string, ok bool) {
	bi, found := debug.ReadBuildInfo()
	if !found {
		return "", false
	}
	mods := append([]*debug.Module{&bi.Main}, bi.Deps...)
	for _, m := range mods {
		if m != nil && m.Path == templaticModule && strings.HasPrefix(m.Version, "v") {
			return m.Version, true
		}
	}
	return "", false
}

func writeScaffold(dir string, data scaffoldData, force bool) ([]string, error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 && !force {
		return nil, fmt.Errorf("init: %s is not empty (use --force to fill in missing files)", dir)
	}
	var written []string
	err := fs.WalkDir(scaffold, "scaffold", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(p, "scaffold/"), ".tmpl")
		if rel == "gitignore" {
			rel = ".gitignore"
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		src, err := scaffold.ReadFile(p)
		if err != nil {
			return err
		}
		tpl, err := template.New(rel).Option("missingkey=error").Parse(string(src))
		if err != nil {
			return fmt.Errorf("init: template %s: %w", rel, err)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, data); err != nil {
			return fmt.Errorf("init: template %s: %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, buf.Bytes(), 0o644); err != nil {
			return err
		}
		written = append(written, filepath.ToSlash(rel))
		return nil
	})
	return written, err
}
