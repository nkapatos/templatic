package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestGoVersionIsTemplaticsOwn(t *testing.T) {
	b, err := os.ReadFile("../../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	f, err := modfile.ParseLax("go.mod", b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Go == nil || f.Go.Version != goVersion {
		t.Fatalf("scaffold go line %q, templatic's go.mod says %v", goVersion, f.Go)
	}
}

func TestModulePathCheck(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	for _, bad := range []string{"has space", "-lead", "a//b", "Ü/x y"} {
		if _, err := newScaffoldData(dir, bad, "v1.0.0", ""); err == nil {
			t.Errorf("module %q must be rejected", bad)
		}
	}
	for _, ok := range []string{"app", "example.com/x/app"} {
		if _, err := newScaffoldData(dir, ok, "v1.0.0", ""); err != nil {
			t.Errorf("module %q: %v", ok, err)
		}
	}
}

func TestDevBuildHasNoReleaseVersion(t *testing.T) {
	if _, ok := binaryVersion(); ok {
		t.Skip("test binary carries a release version")
	}
	dir := filepath.Join(t.TempDir(), "app")
	if _, err := newScaffoldData(dir, "", "", ""); err == nil || !strings.Contains(err.Error(), "no release version") {
		t.Fatalf("err = %v", err)
	}
	d, err := newScaffoldData(dir, "", "", "../../..")
	if err != nil || d.Version != replaceVersion {
		t.Fatalf("replace without a release: %+v %v", d, err)
	}
}
