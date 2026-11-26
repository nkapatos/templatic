package generate

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

func writeClientFile(t *testing.T, opts Options, rel, content string) {
	t.Helper()
	p := filepath.Join(opts.Root, "client", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func manifestAssets(t *testing.T, opts Options) map[string]string {
	t.Helper()
	var m struct {
		Assets map[string]string `json:"assets"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(distOf(opts), "tc.json"))), &m); err != nil {
		t.Fatal(err)
	}
	return m.Assets
}

func TestDefaultAssetLoaders(t *testing.T) {
	opts := testAppOpts(t)
	var imports, uses []string
	for ext := range defaultAssetLoaders {
		name := "f" + strings.TrimPrefix(ext, ".")
		writeClientFile(t, opts, "assets/"+name+ext, "bytes-of-"+ext)
		imports = append(imports, `import `+name+` from "../assets/`+name+ext+`";`)
		uses = append(uses, name)
	}
	setIslandScript(t, opts, strings.Join(imports, "\n")+"\nonMount((el) => { el.dataset.all = [ "+strings.Join(uses, ", ")+" ].join(); })")
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	assets := manifestAssets(t, opts)
	for ext := range defaultAssetLoaders {
		name := "f" + strings.TrimPrefix(ext, ".")
		url := assets["client/assets/"+name+ext]
		if !regexp.MustCompile(`^assets/` + name + `-[0-9A-Za-z]+\` + ext + `$`).MatchString(url) {
			t.Fatalf("%s: assets map %v", ext, assets)
		}
		if got := read(t, filepath.Join(distOf(opts), filepath.FromSlash(url))); got != "bytes-of-"+ext {
			t.Fatalf("%s: bytes %q", ext, got)
		}
	}
}

func TestLoaderOptionOverridesDefault(t *testing.T) {
	opts := testAppOpts(t)
	opts.Loader = map[string]api.Loader{".svg": api.LoaderDataURL}
	writeClientFile(t, opts, "assets/logo.svg", "<svg/>")
	setIslandScript(t, opts, `import logo from "../assets/logo.svg";
onMount((el) => { el.dataset.logo = logo; })`)
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if a := manifestAssets(t, opts); len(a) != 0 {
		t.Fatalf("data URL must not emit a file: %v", a)
	}
	if !strings.Contains(mustIslandChunk(t, opts, "user-card"), "data:image/svg+xml") {
		t.Fatal("svg not inlined as a data URL")
	}
}

func TestImportSuffixes(t *testing.T) {
	opts := testAppOpts(t)
	writeClientFile(t, opts, "assets/query.sql", "SELECT 'raw-marker' FROM t;")
	writeClientFile(t, opts, "assets/doc.pdf", "%PDF-hello")
	writeClientFile(t, opts, "assets/icon.svg", "<svg>inline-marker</svg>")
	setIslandScript(t, opts, `import sql from "../assets/query.sql?raw";
import pdf from "../assets/doc.pdf?url";
import icon from "../assets/icon.svg?inline";
import iconUrl from "../assets/icon.svg?url";
import iconDefault from "../assets/icon.svg";
onMount((el) => { el.dataset.x = [sql, pdf, icon, iconUrl, iconDefault].join("|"); })`)
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	chunk := mustIslandChunk(t, opts, "user-card")
	if !strings.Contains(chunk, "raw-marker") {
		t.Fatal("?raw did not inline the file text")
	}
	if !strings.Contains(chunk, "data:image/svg+xml,<svg>inline-marker</svg>") {
		t.Fatalf("?inline did not produce a data URL: %s", chunk)
	}
	assets := manifestAssets(t, opts)
	pdf := assets["client/assets/doc.pdf"]
	if !regexp.MustCompile(`^assets/doc-[0-9A-Za-z]+\.pdf$`).MatchString(pdf) {
		t.Fatalf("?url of an unknown extension must emit a hashed file keeping its extension: %v", assets)
	}
	if got := read(t, filepath.Join(distOf(opts), filepath.FromSlash(pdf))); got != "%PDF-hello" {
		t.Fatalf("?url bytes: %q", got)
	}
	if assets["client/assets/icon.svg"] == "" {
		t.Fatalf("plain/?url svg import missing from the assets map: %v", assets)
	}
}

func TestImportSuffixMissingFileFailsBuild(t *testing.T) {
	opts := testAppOpts(t)
	setIslandScript(t, opts, `import x from "../assets/nope.svg?url";
onMount((el) => { el.dataset.x = x; })`)
	if _, err := Run(opts); err == nil {
		t.Fatal("a ?url import of a missing file must fail the build")
	}
}

func TestAssetsDeclarationsCopied(t *testing.T) {
	opts, s := runApp(t)
	d := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "assets.d.ts"))
	for ext := range defaultAssetLoaders {
		if !strings.Contains(d, `declare module "*`+ext+`"`) {
			t.Errorf("assets.d.ts does not declare *%s", ext)
		}
	}
	for _, suffix := range []string{"url", "raw", "inline"} {
		if !strings.Contains(d, `declare module "*?`+suffix+`"`) {
			t.Errorf("assets.d.ts does not declare *?%s", suffix)
		}
	}
	found := false
	for _, f := range s.RuntimeFiles {
		found = found || strings.HasSuffix(f, ".templatic/runtime/assets.d.ts")
	}
	if !found {
		t.Fatalf("assets.d.ts missing from Summary.RuntimeFiles: %v", s.RuntimeFiles)
	}
}
