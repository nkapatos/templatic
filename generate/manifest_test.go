package generate

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

// hashName strips content hashes so the golden survives esbuild upgrades.
var hashName = regexp.MustCompile(`-[0-9A-Za-z]{8,10}\.`)

func normalizeManifest(t *testing.T, raw string) string {
	t.Helper()
	return hashName.ReplaceAllString(raw, "-HASH.")
}

func readManifestMap(t *testing.T, opts Options) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(read(t, filepath.Join(distOf(opts), "tc.json"))), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestManifestGolden(t *testing.T) {
	opts, _ := runApp(t)
	got := normalizeManifest(t, read(t, filepath.Join(distOf(opts), "tc.json")))
	golden := filepath.Join("testdata", "golden", "tc.json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if want := read(t, golden); got != want {
		t.Fatalf("tc.json differs from %s (run go test ./generate -update if intended)\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}

func readModules(t *testing.T, opts Options) map[string]ManifestModule {
	t.Helper()
	var m struct {
		Modules map[string]ManifestModule `json:"modules"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(distOf(opts), "tc.json"))), &m); err != nil {
		t.Fatal(err)
	}
	return m.Modules
}

func TestManifestHasNoRemovedKeys(t *testing.T) {
	opts, _ := runApp(t)
	m := readManifestMap(t, opts)
	if m["version"] != float64(2) {
		t.Fatalf("version = %v, want 2", m["version"])
	}
	for _, k := range []string{"islands", "css", "boot", "entries"} {
		if _, ok := m[k]; ok {
			t.Fatalf("removed key %q written to tc.json", k)
		}
	}
	for _, k := range []string{"version", "mode", "base", "modules", "assets", "integrity", "binds", "meta"} {
		delete(m, k)
	}
	if len(m) != 0 {
		t.Fatalf("unexpected keys in tc.json: %v", m)
	}
}

func TestManifestModulesCoverEntries(t *testing.T) {
	opts, _ := runApp(t)
	mods := readModules(t, opts)
	if mods["boot"].File == "" {
		t.Fatalf("modules.boot has no file: %#v", mods["boot"])
	}
	for _, tag := range []string{"user-card", "big-chart"} {
		mod := mods["islands/"+tag]
		if mod.File == "" {
			t.Fatalf("modules[islands/%s] missing: %#v", tag, mods)
		}
		if tag == "user-card" && len(mod.CSS) != 1 {
			t.Fatalf("modules[islands/user-card].css = %v, want one stylesheet", mod.CSS)
		}
		if tag == "big-chart" && len(mod.CSS) != 0 {
			t.Fatalf("island without styles has css %v", mod.CSS)
		}
		for _, f := range append([]string{mod.File}, mod.Imports...) {
			if _, err := os.Stat(filepath.Join(distOf(opts), filepath.FromSlash(f))); err != nil {
				t.Fatalf("module %s references %s: %v", tag, f, err)
			}
		}
		if len(mod.Imports) == 0 {
			t.Fatalf("islands/%s has no chunk imports; the fixture shares a chunk", tag)
		}
	}
}

func out(path string, imports ...string) Output {
	o := Output{Path: path}
	for _, im := range imports {
		o.Imports = append(o.Imports, Import{Path: im, Kind: "import-statement"})
	}
	return o
}

func TestBuildModulesTransitiveImports(t *testing.T) {
	root := "/app"
	entry := out(".templatic/dist/boot.js", ".templatic/dist/chunks/a.js", "https://cdn/x.js")
	entry.Entry = ".templatic/runtime/boot.ts"
	entry.Imports = append(entry.Imports, Import{Path: ".templatic/dist/chunks/lazy.js", Kind: "dynamic-import"})
	entry.CSS = ".templatic/dist/boot.css"
	a := out(".templatic/dist/chunks/a.js", ".templatic/dist/chunks/b.js", ".templatic/dist/chunks/c.js")
	b := out(".templatic/dist/chunks/b.js", ".templatic/dist/chunks/c.js", ".templatic/dist/boot.js")
	c := out(".templatic/dist/chunks/c.js")
	lazy := out(".templatic/dist/chunks/lazy.js", ".templatic/dist/chunks/d.js")
	d := out(".templatic/dist/chunks/d.js")
	outs := []Output{entry, a, b, c, lazy, d}
	table := &Entries{Root: root, List: []Entry{{Name: "boot", Input: root + "/.templatic/runtime/boot.ts"}}}
	mods := buildModules(table, root, ".templatic/dist", outs)
	got := mods["boot"]
	want := ManifestModule{
		File:    "boot.js",
		CSS:     []string{"boot.css"},
		Imports: []string{"chunks/a.js", "chunks/b.js", "chunks/c.js"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("modules[boot]\n got: %#v\nwant: %#v", got, want)
	}
}

func TestManifestAssetsMapSourceToHashedURL(t *testing.T) {
	opts := testAppOpts(t)
	opts.Loader = map[string]api.Loader{".svg": api.LoaderFile}
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`
	if err := os.MkdirAll(filepath.Join(opts.Root, "client", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.Root, "client", "assets", "logo.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	setIslandScript(t, opts, `import logo from "../assets/logo.svg";
onMount((el) => { el.dataset.logo = logo; })`)
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Assets map[string]string `json:"assets"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(distOf(opts), "tc.json"))), &m); err != nil {
		t.Fatal(err)
	}
	url := m.Assets["client/assets/logo.svg"]
	if !regexp.MustCompile(`^assets/logo-[0-9A-Za-z]+\.svg$`).MatchString(url) {
		t.Fatalf("assets map = %#v", m.Assets)
	}
	if got := read(t, filepath.Join(distOf(opts), filepath.FromSlash(url))); got != svg {
		t.Fatalf("asset bytes changed: %q", got)
	}
	if !strings.Contains(mustIslandChunk(t, opts, "user-card"), url) {
		t.Fatalf("island chunk does not reference the hashed asset %s", url)
	}
}

func TestManifestBytesAreDeterministic(t *testing.T) {
	first, _ := runApp(t)
	second, _ := runApp(t)
	a := normalizeManifest(t, read(t, filepath.Join(distOf(first), "tc.json")))
	b := normalizeManifest(t, read(t, filepath.Join(distOf(second), "tc.json")))
	if a != b {
		t.Fatalf("two runs wrote different manifests:\n%s\n%s", a, b)
	}
}

func TestManifestMarshalSortsMapKeys(t *testing.T) {
	m := Manifest{Meta: map[string]any{}}
	for _, k := range []string{"q", "b", "x", "a", "m", "z", "c"} {
		m.Meta[k] = map[string]any{"y": 1, "x": 2, "w": 3}
	}
	want, err := json.Marshal(m, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		got, _ := json.Marshal(m, jsontext.WithIndent("  "), json.Deterministic(true))
		if string(got) != string(want) {
			t.Fatalf("unstable bytes:\n%s\n%s", got, want)
		}
	}
	if strings.Contains(string(want), `"version"`) {
		t.Errorf("a zero Version must be omitted:\n%s", want)
	}
	m.Version = 2
	if got, _ := json.Marshal(m); !strings.Contains(string(got), `"version":2`) {
		t.Errorf("Version 2 missing: %s", got)
	}
}
