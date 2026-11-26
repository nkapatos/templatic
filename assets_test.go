package templatic_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nkapatos/templatic"
)

func TestAssetsCachingFollowsContentAddressing(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"dist/boot.js":                 "boot",
		"dist/tc.json":                 "{}",
		"dist/islands/card-abc123.js":  "island",
		"dist/islands/card-abc123.css": "styles",
		"dist/chunks/chunk-XYZ123.js":  "chunk",
		"app.css":                      "app",
	}
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := templatic.Assets(dir)

	cases := []struct {
		path      string
		immutable bool
	}{
		{"/dist/islands/card-abc123.js", true},
		{"/dist/islands/card-abc123.css", true},
		{"/dist/chunks/chunk-XYZ123.js", true},
		{"/dist/boot.js", false},
		{"/dist/tc.json", false},
		{"/app.css", false},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", c.path, rec.Code)
		}
		got := rec.Header().Get("Cache-Control")
		if c.immutable {
			if got != "public, max-age=31536000, immutable" {
				t.Fatalf("%s: cache-control = %q, want immutable", c.path, got)
			}
		} else if got != "no-cache" {
			t.Fatalf("%s: cache-control = %q, want no-cache (must revalidate so deploys land)", c.path, got)
		}
	}
}

func TestAssetsTreatsHashedBootAsImmutable(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"dist/boot.js", "dist/boot-abc1234567.js"} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := templatic.Assets(dir)
	for path, want := range map[string]string{
		"/dist/boot-abc1234567.js": "public, max-age=31536000, immutable",
		"/dist/boot.js":            "no-cache",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Fatalf("%s: cache-control = %q, want %q", path, got, want)
		}
	}
}

func TestAssetsServesFingerprintedAppAssetImmutable(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "app.css")
	if err := os.WriteFile(css, []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}
	version, err := templatic.AssetFingerprint(dir, "app.css")
	if err != nil || version == "" {
		t.Fatalf("AssetFingerprint: %v (%q)", err, version)
	}
	h := templatic.Assets(dir)

	cases := map[string]string{
		"/app.css?v=" + version: "public, max-age=31536000, immutable",
		"/app.css":              "no-cache",
		"/app.css?v=stalehash":  "no-cache",
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Fatalf("%s: cache-control = %q, want %q", path, got, want)
		}
	}

	if err := os.WriteFile(css, []byte("body{color:blue}"), 0o644); err != nil {
		t.Fatal(err)
	}
	next, err := templatic.AssetFingerprint(dir, "app.css")
	if err != nil {
		t.Fatal(err)
	}
	if next == version {
		t.Fatal("fingerprint must change with content")
	}
	for path, want := range map[string]string{
		"/app.css?v=" + version: "no-cache", // stale URL must revalidate
		"/app.css?v=" + next:    "public, max-age=31536000, immutable",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Fatalf("after rebuild %s: cache-control = %q, want %q", path, got, want)
		}
	}
}

func TestAssetsNeverCachesErrorsImmutable(t *testing.T) {
	dir := t.TempDir()
	h := templatic.Assets(dir)
	for _, p := range []string{
		"/dist/islands/card-0123456789.js",
		"/dist/chunks/chunk-ABCDEF.js",
		"/dist/boot-0123456789.js",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status %d, want 404", p, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s: cache-control on a 404 = %q, want no-store", p, got)
		}
	}
}

func TestAssetsDoesNotListDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dist", "islands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", "islands", "card-0123456789.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := templatic.Assets(dir)
	for _, p := range []string{"/", "/dist/", "/dist", "/dist/islands/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status %d, want 404 (no directory listing)", p, rec.Code)
		}
	}
}
