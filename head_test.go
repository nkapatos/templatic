package templatic_test

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic"
)

var updateHead = flag.Bool("update-head", false, "rewrite head golden files")

func renderHead(t *testing.T, o templatic.HeadOptions) string {
	t.Helper()
	var buf bytes.Buffer
	if err := templatic.Head(o).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *updateHead {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("%s differs (go test -update-head)\n got: %s\nwant: %s", name, got, want)
	}
}

func headManifest() *templatic.Manifest {
	return &templatic.Manifest{
		Version: 2,
		Modules: map[string]templatic.ManifestModule{
			"boot":           {File: "boot-1111111111.js", Imports: []string{"chunks/a-AAAAAAAA.js", "chunks/b-BBBBBBBB.js"}},
			"app":            {File: "app-5555555555.js", CSS: []string{"app-6666666666.css"}, Imports: []string{"chunks/b-BBBBBBBB.js"}},
			"islands/editor": {File: "islands/editor-2222222222.js", CSS: []string{"islands/editor-4444444444.css"}, Imports: []string{"chunks/b-BBBBBBBB.js", "chunks/c-CCCCCCCC.js"}},
			"islands/plain":  {File: "islands/plain-3333333333.js"},
		},
		Integrity: map[string]string{
			"boot-1111111111.js":            "sha384-boot",
			"chunks/b-BBBBBBBB.js":          "sha384-b",
			"app-6666666666.css":            "sha384-appcss",
			"islands/editor-4444444444.css": "sha384-editorcss",
		},
	}
}

func TestHeadGolden(t *testing.T) {
	got := renderHead(t, templatic.HeadOptions{
		Assets:   "/_templatic/",
		Manifest: headManifest(),
		Styles:   []string{"app"},
		Modules:  []string{"app"},
		Preload:  []string{"editor", "plain"},
	})
	golden(t, "head.html", got)
}

func TestHeadOrderAndDedupe(t *testing.T) {
	got := renderHead(t, templatic.HeadOptions{Assets: "/s", Manifest: headManifest(), Modules: []string{"app"}, Preload: []string{"editor"}})
	if n := strings.Count(got, `href="/s/chunks/b-BBBBBBBB.js"`); n != 1 {
		t.Fatalf("shared chunk preloaded %d times, want once: %s", n, got)
	}
	if strings.Index(got, `rel="stylesheet"`) > strings.Index(got, `rel="modulepreload"`) {
		t.Fatalf("stylesheets must precede preloads: %s", got)
	}
	if !strings.Contains(got, `chunks/c-CCCCCCCC.js`) {
		t.Fatalf("transitive chunk missing: %s", got)
	}
}

func TestHeadWithoutManifestWritesNothing(t *testing.T) {
	if got := renderHead(t, templatic.HeadOptions{Assets: "/s", Styles: []string{"app"}}); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestHeadEscapesAttributes(t *testing.T) {
	m := &templatic.Manifest{Modules: map[string]templatic.ManifestModule{"app": {CSS: []string{`a"b<.css`}}}}
	got := renderHead(t, templatic.HeadOptions{Assets: "/s", Manifest: m, Styles: []string{"app"}})
	if strings.Contains(got, `a"b<`) || !strings.Contains(got, "a&quot;b&lt;.css") {
		t.Fatalf("not escaped: %s", got)
	}
}

func TestHeadWritesTemplNonceOnModulePreloads(t *testing.T) {
	var buf bytes.Buffer
	ctx := templ.WithNonce(context.Background(), "n1")
	if err := templatic.Head(templatic.HeadOptions{Assets: "/s", Manifest: headManifest(), Styles: []string{"app"}}).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, `<link rel="modulepreload" href="/s/boot-1111111111.js" nonce="n1" integrity="sha384-boot">`) {
		t.Fatalf("modulepreload lacks the nonce: %s", got)
	}
	if strings.Contains(got, `rel="stylesheet" href="/s/app-6666666666.css" nonce`) {
		t.Fatalf("stylesheets take no script nonce: %s", got)
	}
}
