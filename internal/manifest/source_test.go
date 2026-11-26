package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nkapatos/templatic/internal/buildfiles"
	"github.com/nkapatos/templatic/live"
)

func mf(boot, appCSS, islandJS string) *Manifest {
	return &Manifest{
		Version: 2,
		Modules: map[string]Module{
			"boot":           {File: boot},
			"app":            {File: "app-1.js", CSS: []string{appCSS}},
			"islands/x-card": {File: islandJS},
		},
	}
}

func TestDevChangeClassifies(t *testing.T) {
	base := mf("boot-1.js", "app-1.css", "x-1.js")
	if f, ok := devChange(base, mf("boot-1.js", "app-1.css", "x-1.js")); ok {
		t.Fatalf("no change sent %+v", f)
	}
	f, ok := devChange(base, mf("boot-1.js", "app-2.css", "x-1.js"))
	if !ok || f.What != "css" || !reflect.DeepEqual(f.CSS, map[string]string{"app-1.css": "app-2.css"}) {
		t.Fatalf("css-only change = %+v", f)
	}
	for _, cur := range []*Manifest{mf("boot-2.js", "app-1.css", "x-1.js"), mf("boot-1.js", "app-2.css", "x-2.js")} {
		if f, ok := devChange(base, cur); !ok || f.What != "reload" {
			t.Fatalf("js change = %+v", f)
		}
	}
	if f, _ := devChange(nil, base); f.What != "reload" {
		t.Fatal("first manifest should reload")
	}
}

func writeTC(t *testing.T, dir, body string) {
	t.Helper()
	p := filepath.Join(dir, "tc.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Duration(len(body)) * time.Second)
	_ = os.Chtimes(p, now, now)
}

func TestManifestForProdAndDev(t *testing.T) {
	dir := t.TempDir()
	if _, err := For("prod", dir); err == nil {
		t.Fatal("prod without tc.json must fail")
	}
	s, err := For("dev", dir)
	if err != nil || s.Current() != nil || !s.Dev() {
		t.Fatalf("dev without tc.json: %v %v", s, err)
	}
	writeTC(t, dir, `{"version":2,"modules":{"boot":{"file":"boot-1.js"}}}`)
	s.poll()
	if s.Current() == nil || s.Current().Modules["boot"].File != "boot-1.js" {
		t.Fatalf("dev did not pick up tc.json: %+v", s.Current())
	}
	writeTC(t, dir, `{"version":2,"modules":{"boot":{"file":"boot-22.js"}}}`)
	s.poll()
	if s.Current().Modules["boot"].File != "boot-22.js" {
		t.Fatal("dev did not re-read tc.json")
	}
	p, err := For("prod", dir)
	if err != nil || p.Current().Modules["boot"].File != "boot-22.js" || p.Dev() {
		t.Fatalf("prod: %+v %v", p, err)
	}
	p.Notify(live.NewWorld()) // no-op, no watcher
	if p.started {
		t.Fatal("prod source started a watcher")
	}
}

func TestDevErrorFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeTC(t, dir, `{"version":2,"modules":{"boot":{"file":"boot-1.js"}}}`)
	s, _ := For("dev", dir)
	_ = os.WriteFile(filepath.Join(dir, buildfiles.DevError), []byte(`{"message":"x.ts:3:1: bad"}`), 0o644)
	s.poll()
	if s.errMsg != "x.ts:3:1: bad" {
		t.Fatalf("errMsg = %q", s.errMsg)
	}
	_ = os.Remove(filepath.Join(dir, buildfiles.DevError))
	s.poll()
	if s.errMsg != "" {
		t.Fatal("error not cleared")
	}
}
