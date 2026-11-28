package templatic_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/generate"
	"github.com/nkapatos/templatic/internal/testapp"
)

func buildAssetApp(t *testing.T, mode generate.Mode) string {
	t.Helper()
	root := t.TempDir()
	testapp.Copy(t, root)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "client", "assets"), 0o755))
	must(os.WriteFile(filepath.Join(root, "client", "assets", "logo.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644))
	card := filepath.Join(root, "widgets", "card.templ")
	b, err := os.ReadFile(card)
	must(err)
	patched := strings.Replace(string(b), "<script>", "<script>\n\t\timport logo from \"../assets/logo.svg\";\n\t\tonMount((el) => { el.dataset.logo = logo; });", 1)
	must(os.WriteFile(card, []byte(patched), 0o644))
	if _, err := generate.Run(generate.Options{Root: root, Mode: mode}); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, ".templatic", "dist")
}

func TestAssetResolvesHashedURLServedImmutable(t *testing.T) {
	dist := buildAssetApp(t, generate.ModeProd)
	m, err := templatic.LoadManifest(dist)
	if err != nil {
		t.Fatal(err)
	}
	templatic.UseManifest(m)
	t.Cleanup(func() { templatic.UseManifest(nil) })

	url := templatic.Asset(context.Background(), "client/assets/logo.svg")
	if !regexp.MustCompile(`^/_templatic/assets/logo-[A-Z0-9]{8}\.svg$`).MatchString(url) {
		t.Fatalf("Asset = %q, want a hashed URL under the mount base", url)
	}
	for _, spelling := range []string{"./client/assets/logo.svg", "/client/assets/logo.svg", "client/assets/../assets/logo.svg"} {
		if got := templatic.Asset(context.Background(), spelling); got != url {
			t.Fatalf("Asset(%q) = %q, want %q", spelling, got, url)
		}
	}
	chunk, err := os.ReadFile(filepath.Join(dist, filepath.FromSlash(m.Modules["islands/user-card"].File)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(chunk, []byte(url)) {
		t.Fatalf("island chunk does not reference %s", url)
	}

	h := http.StripPrefix("/_templatic/", templatic.Assets(dist))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d", url, rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache-control = %q, want immutable", got)
	}
}

func TestAssetUnknownPathDevPanicsProdLogs(t *testing.T) {
	t.Run("dev panics naming the known assets", func(t *testing.T) {
		m := &templatic.Manifest{Mode: "dev", Base: "/_templatic", Assets: map[string]string{"client/a.svg": "assets/a-AAAAAAAA.svg"}}
		defer func() {
			msg, _ := recover().(string)
			if !strings.Contains(msg, `unknown asset "client/b.svg"`) || !strings.Contains(msg, "client/a.svg") {
				t.Fatalf("panic = %q", msg)
			}
		}()
		_ = m.Asset("client/b.svg")
		t.Fatal("dev manifest must panic on an unknown asset")
	})
	t.Run("prod logs an error and returns empty", func(t *testing.T) {
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })
		m := &templatic.Manifest{Base: "/_templatic", Assets: map[string]string{"client/a.svg": "assets/a-AAAAAAAA.svg"}}
		if got := m.Asset("client/b.svg"); got != "" {
			t.Fatalf("prod unknown asset = %q, want empty", got)
		}
		if !strings.Contains(buf.String(), "level=ERROR") || !strings.Contains(buf.String(), "client/b.svg") {
			t.Fatalf("no error logged: %q", buf.String())
		}
	})
	t.Run("no registered manifest panics in either mode", func(t *testing.T) {
		templatic.UseManifest(nil)
		defer func() {
			if recover() == nil {
				t.Fatal("Asset before UseManifest must panic")
			}
		}()
		templatic.Asset(context.Background(), "client/a.svg")
	})
}

func TestAssetTrimsTrailingSlashOfBase(t *testing.T) {
	m := &templatic.Manifest{Base: "/_templatic/", Assets: map[string]string{"client/a.svg": "assets/a-AAAAAAAA.svg"}}
	if got := m.Asset("client/a.svg"); got != "/_templatic/assets/a-AAAAAAAA.svg" {
		t.Fatalf("got %q", got)
	}
}

func TestAssetsHandlerOnlyHashedBundledAssetsImmutable(t *testing.T) {
	dir := t.TempDir()
	for rel, want := range map[string]bool{
		"dist/assets/logo-AB12CD34.svg":   true,
		"dist/assets/font-ZZ99YY88.woff2": true,
		"assets/logo.svg":                 false,
		"assets/logo-v2.svg":              false,
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		templatic.Assets(dir).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+rel, nil))
		got := rec.Header().Get("Cache-Control") == "public, max-age=31536000, immutable"
		if got != want {
			t.Errorf("%s: immutable=%v, want %v", rel, got, want)
		}
	}
}

func TestAssetReadsTheRequestsManifest(t *testing.T) {
	dist := buildAssetApp(t, generate.ModeProd)
	src, err := templatic.ManifestFor("prod", dist)
	if err != nil {
		t.Fatal(err)
	}
	templatic.UseManifest(nil) // only the request's manifest can answer
	var got string
	h := templatic.WithAssets(src)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = templatic.Asset(r.Context(), "client/assets/logo.svg")
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !regexp.MustCompile(`^/_templatic/assets/logo-[A-Z0-9]{8}\.svg$`).MatchString(got) {
		t.Fatalf("Asset = %q, want the hashed URL from the request's manifest", got)
	}
}
