package generate

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/internal/testapp"
)

func entriesOf(t *testing.T, root string) map[string]ManifestModule {
	t.Helper()
	return readModules(t, Options{Root: root})
}

func TestClientOnlyEntriesByConvention(t *testing.T) {
	opts := testAppOpts(t)
	client := filepath.Join(opts.Root, "client")
	if err := os.MkdirAll(filepath.Join(client, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"app.ts":             "import { on } from \"templatic\";\non(\"ready\", () => console.log('app-entry'));\n",
		"app.css":            ".app{color:red}\n",
		"modules/charts.ts":  "console.log('charts-module');\nexport {};\n",
		"modules/types.d.ts": "export {};\n",
	} {
		if err := os.WriteFile(filepath.Join(client, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(client, "app.ts"), []byte("import './app.css';\nimport { on } from \"templatic\";\non(\"ready\", () => console.log('app-entry'));\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	e := entriesOf(t, opts.Root)
	for key, pat := range map[string]string{
		"app":            `^app-[0-9a-f]{10}\.js$`,
		"modules/charts": `^modules/charts-[0-9a-f]{10}\.js$`,
	} {
		if !regexp.MustCompile(pat).MatchString(e[key].File) {
			t.Errorf("modules[%q].file = %q, want %s", key, e[key].File, pat)
		}
	}
	if css := e["app"].CSS; len(css) != 1 || !regexp.MustCompile(`^app-[0-9a-f]{10}\.css$`).MatchString(css[0]) {
		t.Errorf("modules[app].css = %v, want one hashed stylesheet", css)
	}
	if _, ok := e["modules/types.d"]; ok {
		t.Error(".d.ts files must not become entries")
	}
}

func TestNoClientOnlyEntriesWithoutFiles(t *testing.T) {
	opts := testAppOpts(t)
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	for name := range entriesOf(t, opts.Root) {
		if name == "app" || strings.HasPrefix(name, "modules/") {
			t.Fatalf("absent app.ts/modules must add nothing, got %q", name)
		}
	}
}

// filepath.Glob read '[' in the root as a pattern and matched nothing.
func TestClientModulesUnderAGlobLikeRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app[1]")
	testapp.Copy(t, root)
	mods := filepath.Join(root, "client", "modules")
	if err := os.MkdirAll(mods, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mods, "charts.ts"), []byte("export {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	if e := entriesOf(t, root); e["modules/charts"].File == "" {
		t.Fatalf("modules/charts missing from entries: %v", e)
	}
}
