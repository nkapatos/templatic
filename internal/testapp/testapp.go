// Package testapp is the fixture app of the generate tests, shared by the packages that run generate over it.
// It copies generate/testdata/app into a temporary module; only tests import it.
package testapp

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Copy writes the fixture app into dst as a Go module of its own.
func Copy(t testing.TB, dst string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(file), "..", "..")
	src := filepath.Join(repo, "generate", "testdata", "app")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	WriteModule(t, dst, "testdata.example/templatic-app")
}

// WriteModule makes dir the module named module, resolving templ and this checkout of templatic without the network.
func WriteModule(t testing.TB, dir, module string) {
	t.Helper()
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOWORK", "off")
	_, file, _, _ := runtime.Caller(0)
	repo, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", "github.com/a-h/templ")
	cmd.Dir = repo
	ver, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(filepath.Join(repo, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	mod := "module " + module + "\n\ngo 1.27.4\n\nrequire (\n\tgithub.com/a-h/templ " + strings.TrimSpace(string(ver)) +
		"\n\tgithub.com/nkapatos/templatic v0.0.0\n)\n\nreplace github.com/nkapatos/templatic => " + repo + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644); err != nil {
		t.Fatal(err)
	}
}
