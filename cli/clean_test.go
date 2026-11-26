package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/generate"
)

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCleanRemovesMarkedDir(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, generate.GenDir, generate.MarkerFile), "{}")
	write(t, filepath.Join(root, generate.GenDir, "dist", "tc.json"), "{}")
	write(t, filepath.Join(root, "client", "app.ts"), "//")
	var out, errb bytes.Buffer
	if c := Main([]string{"clean", "-root", root}, &out, &errb); c != 0 {
		t.Fatalf("exit %d: %s", c, errb.String())
	}
	if _, err := os.Stat(filepath.Join(root, generate.GenDir)); !os.IsNotExist(err) {
		t.Fatalf("%s not removed: %v", generate.GenDir, err)
	}
	if _, err := os.Stat(filepath.Join(root, "client", "app.ts")); err != nil {
		t.Fatalf("app file removed: %v", err)
	}
}

func TestCleanRefusesWithoutMarker(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, generate.GenDir, "mine.txt")
	write(t, keep, "x")
	var out, errb bytes.Buffer
	if c := Main([]string{"clean", "-root", root}, &out, &errb); c == 0 || !strings.Contains(errb.String(), "was not created by templatic") {
		t.Fatalf("exit %d, stderr %q", c, errb.String())
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unmarked dir was touched: %v", err)
	}
}

func TestCleanWithoutDirIsNoop(t *testing.T) {
	var out, errb bytes.Buffer
	if c := Main([]string{"clean", "-root", t.TempDir()}, &out, &errb); c != 0 {
		t.Fatalf("exit %d: %s", c, errb.String())
	}
}
