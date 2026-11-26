package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/generate"
	"github.com/nkapatos/templatic/internal/testapp"
)

func environ() []string {
	return []string{"PUBLIC_API_URL=https://x.test/a b", "PUBLIC_QUOTE=say \"hi\"", "SECRET_TOKEN=hunter2", "PUBLIC_=empty-name", "HOME=/root"}
}

func TestDefinesOnlyExplicitPrefix(t *testing.T) {
	root := fixture(t)
	opts := generate.Options{Root: root, SkipBundle: true,
		Defines: map[string]string{"process.env.PUBLIC_API_URL": `"explicit"`},
		Plugins: []generate.Plugin{New("PUBLIC_", Lookup(environ))}}
	var got map[string]string
	opts.Plugins = append(opts.Plugins, hookProbe{func(o *generate.Options) { got = o.Defines }})
	if _, err := generate.Run(opts); err != nil {
		t.Fatal(err)
	}
	if got["process.env.PUBLIC_API_URL"] != `"explicit"` {
		t.Fatalf("explicit define must win: %v", got)
	}
	if got["import.meta.env.PUBLIC_API_URL"] != `"https://x.test/a b"` || got["process.env.PUBLIC_QUOTE"] != `"say \"hi\""` {
		t.Fatalf("PUBLIC_ vars not exposed as JSON strings: %v", got)
	}
	for k := range got {
		if strings.Contains(k, "SECRET") || strings.Contains(k, "HOME") || k == "process.env.PUBLIC_" {
			t.Fatalf("leaked or malformed define %q", k)
		}
	}
}

func TestEmptyPrefixRefused(t *testing.T) {
	_, err := generate.Run(generate.Options{Root: fixture(t), SkipBundle: true,
		Plugins: []generate.Plugin{New("", Lookup(environ))}})
	if err == nil || !strings.HasPrefix(err.Error(), "plugin env: ") {
		t.Fatalf("empty prefix must be refused, attributed: %v", err)
	}
}

func TestReachesTheBundle(t *testing.T) {
	root := fixture(t)
	card := filepath.Join(root, "widgets", "card.templ")
	b, _ := os.ReadFile(card)
	src := string(b)
	i, j := strings.Index(src, "<script>"), strings.Index(src, "</script>")
	src = src[:i] + "<script>\nonMount((el) => { el.dataset.api = process.env.PUBLIC_API_URL; el.dataset.s = String(process.env.SECRET_TOKEN); })\n\t" + src[j:]
	if err := os.WriteFile(card, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := generate.Run(generate.Options{Root: root, Plugins: []generate.Plugin{New("PUBLIC_", Lookup(environ))}}); err != nil {
		t.Fatal(err)
	}
	var found string
	_ = filepath.Walk(filepath.Join(root, ".templatic", "dist", "islands"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".js") && strings.Contains(filepath.Base(p), "user-card") {
			d, _ := os.ReadFile(p)
			found = string(d)
		}
		return nil
	})
	if !strings.Contains(found, "https://x.test/a b") {
		t.Fatalf("PUBLIC_ value missing from the island bundle")
	}
	if strings.Contains(found, "hunter2") {
		t.Fatal("non-prefixed secret leaked into the bundle")
	}
}

type hookProbe struct{ f func(*generate.Options) }

func (hookProbe) Name() string { return "probe" }
func (h hookProbe) Setup(b *generate.Builder) error {
	b.OnConfig(func(o *generate.Options) error { h.f(o); return nil })
	return nil
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testapp.Copy(t, root)
	return root
}
