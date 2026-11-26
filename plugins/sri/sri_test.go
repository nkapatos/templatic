package sri

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/generate"
	"github.com/nkapatos/templatic/internal/testapp"
)

func TestIntegrityKnownVector(t *testing.T) {
	sum := sha512.Sum384([]byte("abc"))
	want := "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
	if got := Integrity([]byte("abc")); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if !strings.HasPrefix(Integrity([]byte("abc")), "sha384-ywB1P0WjXou1oD1pmsZQBycsMqsO3tFjGotgWkP/W+2AhgcroefMI1i67KE0yCWn") {
		t.Fatal("not the standard SHA-384 of abc")
	}
}

func TestManifestCarriesIntegrityForEveryScriptAndStyle(t *testing.T) {
	root := fixture(t)
	if _, err := generate.Run(generate.Options{Root: root, Plugins: []generate.Plugin{New()}}); err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(root, ".templatic", "dist")
	raw, err := os.ReadFile(filepath.Join(dist, "tc.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Integrity map[string]string `json:"integrity"`
		Modules   map[string]struct {
			File string   `json:"file"`
			CSS  []string `json:"css"`
		} `json:"modules"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	card := m.Modules["islands/user-card"]
	want := append([]string{m.Modules["boot"].File, card.File}, card.CSS...)
	for _, rel := range want {
		got, ok := m.Integrity[rel]
		if !ok {
			t.Fatalf("no integrity for %s: %v", rel, m.Integrity)
		}
		data, err := os.ReadFile(filepath.Join(dist, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if got != Integrity(data) {
			t.Fatalf("%s: integrity does not match the bytes on disk", rel)
		}
	}
	for rel := range m.Integrity {
		if !strings.HasSuffix(rel, ".js") && !strings.HasSuffix(rel, ".css") {
			t.Fatalf("unexpected integrity key %s", rel)
		}
	}
	if len(m.Integrity) < 4 {
		t.Fatalf("too few entries: %v", m.Integrity)
	}
}

func TestNoIntegrityWithoutPlugin(t *testing.T) {
	root := fixture(t)
	if _, err := generate.Run(generate.Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".templatic", "dist", "tc.json"))
	if strings.Contains(string(raw), "integrity") {
		t.Fatal("integrity must be absent without the plugin")
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testapp.Copy(t, root)
	return root
}
