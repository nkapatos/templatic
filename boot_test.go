package templatic_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestBootEmitsConfigAndModule(t *testing.T) {
	got := render(t, templatic.Boot(templatic.BootOptions{
		Live:   "/live/",
		Assets: "/_templatic/",
	}))

	if !strings.Contains(got, `<script type="application/tc-config">`) {
		t.Fatalf("config script missing: %q", got)
	}
	start := strings.Index(got, `<script type="application/tc-config">`) + len(`<script type="application/tc-config">`)
	end := strings.Index(got, "</script><script type=\"module\"")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(got[start:end]), &cfg); err != nil {
		t.Fatalf("config payload not valid JSON: %q", got)
	}
	if cfg["stream"] != "/live/stream" || cfg["commands"] != "/live/commands" || cfg["assets"] != "/_templatic" {
		t.Fatalf("config values wrong: %#v", cfg)
	}
	if !strings.Contains(got, `</script><script type="module" src="/_templatic/boot.js" async></script>`) {
		t.Fatalf("module script wrong: %q", got)
	}
}

func TestBootCarriesPageTokenInConfigOnlyWhenSet(t *testing.T) {
	cfgOf := func(o templatic.BootOptions) map[string]any {
		got := render(t, templatic.Boot(o))
		start := strings.Index(got, `<script type="application/tc-config">`) + len(`<script type="application/tc-config">`)
		end := strings.Index(got, "</script><script type=\"module\"")
		var cfg map[string]any
		if err := json.Unmarshal([]byte(got[start:end]), &cfg); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	if _, ok := cfgOf(templatic.BootOptions{Live: "/live", Assets: "/a"})["token"]; ok {
		t.Fatal("empty Token must not appear in the config (byte-identical to before)")
	}
	if got := cfgOf(templatic.BootOptions{Live: "/live", Assets: "/a", Token: "p.s"})["token"]; got != "p.s" {
		t.Fatalf("token = %v", got)
	}
}

func TestBootInlinesManifestAndEmitsPreloads(t *testing.T) {
	m := &templatic.Manifest{
		Version: 2,
		Modules: map[string]templatic.ManifestModule{
			"boot":          {File: "boot-1.js"},
			"modules/chart": {File: "modules/chart-abc.js", Imports: []string{"chunks/c-1.js"}},
			"modules/other": {File: "modules/other-def.js"},
		},
		Integrity: map[string]string{"modules/chart-abc.js": "sha384-chart"},
		Assets:    map[string]string{"img/logo.png": "img/logo-1.png"},
	}
	got := render(t, templatic.Boot(templatic.BootOptions{
		Live:     "/live",
		Assets:   "/_templatic",
		Manifest: m,
		Modules:  []string{"chart", "not-in-manifest"},
	}))
	js := strings.Index(got, `<link rel="modulepreload" href="/_templatic/modules/chart-abc.js" integrity="sha384-chart">`)
	cfg := strings.Index(got, `<script type="application/tc-config">`)
	boot := strings.Index(got, `<script type="module" src="/_templatic/boot-1.js"`)
	if js == -1 || !strings.Contains(got, `href="/_templatic/chunks/c-1.js"`) {
		t.Fatalf("preload hints missing: %q", got)
	}
	if !(js < cfg && cfg < boot) {
		t.Fatalf("preload hints must precede the config and boot script: %q", got)
	}
	if strings.Contains(got, `<link rel="modulepreload" href="/_templatic/modules/other-def.js"`) {
		t.Fatal("only requested modules may be preloaded")
	}
	if !strings.Contains(got, `"manifest"`) || !strings.Contains(got, `"modules":["chart","not-in-manifest"]`) {
		t.Fatalf("manifest and modules not in the page config: %q", got)
	}
	if strings.Contains(got, `"integrity":`) || strings.Contains(got, "img/logo") {
		t.Fatalf("the inlined manifest carries only what the runtime reads: %q", got)
	}
}

func TestBootWithoutManifestKeepsFetchPath(t *testing.T) {
	got := render(t, templatic.Boot(templatic.BootOptions{Live: "/live", Assets: "/_templatic"}))
	if strings.Contains(got, "modulepreload") || strings.Contains(got, `"manifest"`) {
		t.Fatalf("no manifest means no preloads and no inlined manifest: %q", got)
	}
}

func TestBootPrefetchEmitsSpeculationRules(t *testing.T) {
	on := render(t, templatic.Boot(templatic.BootOptions{Live: "/live", Assets: "/_templatic", Prefetch: true}))
	if !strings.Contains(on, `type="speculationrules"`) || !strings.Contains(on, `"eagerness":"moderate"`) {
		t.Fatalf("prefetch must emit speculation rules: %q", on)
	}
	off := render(t, templatic.Boot(templatic.BootOptions{Live: "/live", Assets: "/_templatic"}))
	if strings.Contains(off, "speculationrules") {
		t.Fatal("speculation rules must be opt-in")
	}
}

func TestBootKeepsScriptEndInsideTheConfigTag(t *testing.T) {
	got := render(t, templatic.Boot(templatic.BootOptions{
		Live:   `/</script><script>alert(1)</script>`,
		Assets: "/a",
	}))
	if strings.Contains(got, "<script>alert(1)") || strings.Count(got, "</script>") != 2 {
		t.Fatalf("config string closed the script tag: %q", got)
	}
	if !strings.Contains(got, `</script>`) {
		t.Fatalf("config string not escaped: %q", got)
	}
}

func TestBootWritesTemplNonce(t *testing.T) {
	m := &templatic.Manifest{Modules: map[string]templatic.ManifestModule{
		"boot":          {File: "boot-1.js"},
		"modules/chart": {File: "modules/chart-abc.js"},
	}}
	var buf bytes.Buffer
	ctx := templ.WithNonce(context.Background(), "n1")
	o := templatic.BootOptions{Live: "/live", Assets: "/a", Manifest: m, Prefetch: true, Modules: []string{"chart"}}
	if err := templatic.Boot(o).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		`<link rel="modulepreload" href="/a/modules/chart-abc.js" nonce="n1">`,
		`<script type="speculationrules" nonce="n1">`,
		`<script type="module" src="/a/boot-1.js" nonce="n1" async>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %q", want, got)
		}
	}
	if plain := render(t, templatic.Boot(o)); strings.Contains(plain, "nonce") {
		t.Fatalf("no nonce in context, none written: %q", plain)
	}
}
