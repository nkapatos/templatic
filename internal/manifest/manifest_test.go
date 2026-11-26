package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tc.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const testManifest = `{
  "version": 2,
  "mode": "dev",
  "base": "/_templatic",
  "modules": {
    "boot": {"file": "boot-cccccccccc.js", "imports": ["chunks/x-1111111111.js", "chunks/y-2222222222.js"]},
    "islands/user-card": {"file": "islands/user-card-aaaaaaaaaa.js", "css": ["islands/user-card-bbbbbbbbbb.css"]}
  },
  "assets": {"client/assets/logo.svg": "assets/logo-dddddddd.svg"},
  "integrity": {"boot-cccccccccc.js": "sha384-abc"},
  "binds": {"big-chart": [{"topic": "chart.focus", "field": "focus"}]}
}`

func TestLoadManifestFields(t *testing.T) {
	m, err := Load(writeManifest(t, testManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != 2 || !m.Dev() || m.Base != "/_templatic" {
		t.Fatalf("header: %#v", m)
	}
	if got := m.Modules["boot"].Imports; !reflect.DeepEqual(got, []string{"chunks/x-1111111111.js", "chunks/y-2222222222.js"}) {
		t.Fatalf("boot imports = %v", got)
	}
	if m.Assets["client/assets/logo.svg"] != "assets/logo-dddddddd.svg" || m.Integrity["boot-cccccccccc.js"] != "sha384-abc" {
		t.Fatalf("assets/integrity: %#v %#v", m.Assets, m.Integrity)
	}
	if m.Modules["islands/user-card"].File == "" || len(m.Binds["big-chart"]) != 1 {
		t.Fatalf("modules/binds lost: %#v", m)
	}
}

func TestLoadRefusesOtherVersions(t *testing.T) {
	for name, body := range map[string]string{
		"version 1":       `{"version": 1, "islands": {}}`,
		"missing version": `{"islands": {"user-card": "islands/user-card-aaaaaaaaaa.js"}, "boot": "boot-1.js"}`,
	} {
		_, err := Load(writeManifest(t, body))
		if err == nil || !strings.Contains(err.Error(), "templatic generate") {
			t.Fatalf("%s: err = %v, want one naming templatic generate", name, err)
		}
	}
}

func TestLoadManifestRejectsGarbage(t *testing.T) {
	if _, err := Load(writeManifest(t, `{"modules": 3}`)); err == nil {
		t.Fatal("want a parse error")
	}
}
