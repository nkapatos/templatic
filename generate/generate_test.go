package generate

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/nkapatos/templatic/internal/testapp"
)

func runApp(t *testing.T) (Options, *Summary) {
	t.Helper()
	root := t.TempDir()
	testapp.Copy(t, root)
	opts := Options{Root: root}
	s, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	return opts, s
}

func testAppOpts(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	testapp.Copy(t, root)
	return Options{Root: root}
}

func setIslandScript(t *testing.T, opts Options, body string) {
	t.Helper()
	path := filepath.Join(opts.Root, "widgets", "card.templ")
	src := read(t, path)
	start := strings.Index(src, "<script>")
	end := strings.Index(src, "</script>")
	if start < 0 || end < start {
		t.Fatal("fixture island has no script block")
	}
	out := src[:start] + "<script>\n" + body + "\n\t</script>" + src[end+len("</script>"):]
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunEmitsGoForAllTemplFiles(t *testing.T) {
	_, s := runApp(t)
	var sawCard, sawPlain bool
	for _, emitted := range s.Emitted {
		switch emitted {
		case "widgets/card_templ.go":
			sawCard = true
		case "widgets/plain_templ.go":
			sawPlain = true
		}
	}
	if !sawCard || !sawPlain {
		t.Fatalf("missing emissions: %v", s.Emitted)
	}
}

func TestTakeoverStripsScriptAndStyleFromEmittedGo(t *testing.T) {
	opts, _ := runApp(t)
	goFile := read(t, filepath.Join(opts.Root, "widgets", "card_templ.go"))
	if strings.Contains(goFile, "mounted") {
		t.Fatal("SFC script leaked into emitted Go")
	}
	if strings.Contains(goFile, "royalblue") {
		t.Fatal("SFC style leaked into emitted Go")
	}
	if !strings.Contains(goFile, "card") {
		t.Fatal("island markup missing from emitted Go")
	}
	plainGo := read(t, filepath.Join(opts.Root, "widgets", "plain_templ.go"))
	if !strings.Contains(plainGo, "Plain") {
		t.Fatal("plain pass-through emission missing component")
	}
}

func TestIslandsDiscoveredWithVariants(t *testing.T) {
	_, s := runApp(t)
	if len(s.Islands) != 2 {
		t.Fatalf("expected 2 islands, got %v", s.Islands)
	}
}

func TestPropsTypesGeneratedFromGoStructs(t *testing.T) {
	opts, _ := runApp(t)
	types := read(t, filepath.Join(opts.Root, ".templatic", "islands", "user-card.types.ts"))
	for _, want := range []string{
		"export interface CardProps {",
		"name: string;",
		"count: number;",
		"ratio: number;",
		`inner: Address;`,
		"tags: string[];",
		"Record<string, Address>;",
		"when: string;",
		"export interface Address {",
		"zip: number;",
	} {
		if !strings.Contains(types, want) {
			t.Fatalf("types file missing %q:\n%s", want, types)
		}
	}
	if strings.Contains(types, "Ignored") || strings.Contains(types, "ignored") {
		t.Fatal("json:\"-\" field not skipped")
	}
	if strings.Contains(types, "anon") {
		t.Fatal("unexported field not skipped")
	}
}

func TestHooksModuleCarriesSFCScriptAndCSS(t *testing.T) {
	opts, _ := runApp(t)
	hooks := read(t, filepath.Join(opts.Root, ".templatic", "islands", "user-card.hooks.ts"))
	if !strings.Contains(hooks, "onMount((el, props) =>") {
		t.Fatalf("SFC script not carried verbatim:\n%s", hooks)
	}
	if !strings.Contains(hooks, `import "./user-card.css";`) {
		t.Fatal("css chunk not imported by hooks module")
	}

	litHooks := read(t, filepath.Join(opts.Root, ".templatic", "islands", "big-chart.hooks.ts"))
	if !strings.Contains(litHooks, `import type { LitElement } from "lit";`) {
		t.Fatalf("lit variant hooks missing typed import:\n%s", litHooks)
	}
	if !strings.Contains(litHooks, `usedHelper(el);`) {
		t.Fatal("lit variant SFC script not carried")
	}

	css := read(t, filepath.Join(opts.Root, ".templatic", "islands", "user-card.css"))
	if !strings.Contains(css, "royalblue") {
		t.Fatalf("css chunk missing style content: %q", css)
	}
}

func TestLitBaseClearsSSRPlaceholderOnce(t *testing.T) {
	// lit's render leaves placeholder children in a light-DOM host; the base clears them once per host.
	opts, _ := runApp(t)
	litBase := read(t, filepath.Join(opts.Root, ".templatic", "islands", "big-chart.base.ts"))
	if !strings.Contains(litBase, "if (!this._tcHydrated) {") {
		t.Fatal("lit base missing the placeholder-clear hydration guard (exact: 'if (!this._tcHydrated)')")
	}
	if !strings.Contains(litBase, "child.remove()") {
		t.Fatal("placeholder clear must remove the SSR children")
	}
	// The literal must not reach the base; TestSharedChunksDeDuplicate counts it.
	if i, j := strings.Index(litBase, "readProps<"), strings.Index(litBase, "child.remove()"); i < 0 || j < 0 || i > j {
		t.Fatal("base must readProps before clearing the placeholder children")
	}
	if strings.Contains(litBase, "application/tc-props") {
		t.Fatal("base must not carry the tc-props literal (readProps owns it; the marker is the shared-chunk contract)")
	}
	hooksBase := read(t, filepath.Join(opts.Root, ".templatic", "islands", "user-card.base.ts"))
	if strings.Contains(hooksBase, "_tcHydrated") || strings.Contains(hooksBase, "child.remove()") {
		t.Fatal("hooks island base must keep the server-rendered markup")
	}
}

func TestBaseClassesMatchVariants(t *testing.T) {
	opts, _ := runApp(t)
	hooksBase := read(t, filepath.Join(opts.Root, ".templatic", "islands", "user-card.base.ts"))
	if !strings.Contains(hooksBase, "export class UserCardBase extends HTMLElement {") {
		t.Fatalf("hooks variant base wrong:\n%s", hooksBase)
	}
	litBase := read(t, filepath.Join(opts.Root, ".templatic", "islands", "big-chart.base.ts"))
	if !strings.Contains(litBase, "export class BigChartBase extends LitElement {") {
		t.Fatalf("lit variant base wrong:\n%s", litBase)
	}
	if !strings.Contains(litBase, "static properties") {
		t.Fatal("lit variant missing reactive props declaration")
	}
}

func TestDefineStubsGeneratedOnceAndPreserved(t *testing.T) {
	opts, _ := runApp(t)
	define := filepath.Join(opts.Root, "client", "islands", "user-card.ts")
	defineSrc := read(t, define)
	if !strings.Contains(defineSrc, "customElements.define(") || !strings.Contains(defineSrc, `"user-card"`) {
		t.Fatalf("define stub wrong:\n%s", defineSrc)
	}

	if err := os.WriteFile(define, []byte("// user-owned now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if got := read(t, define); !strings.Contains(got, "// user-owned now") {
		t.Fatal("define stub was regenerated over user content")
	}
	if !strings.Contains(read(t, filepath.Join(opts.Root, ".templatic", "islands", "user-card.base.ts")), "UserCardBase") {
		t.Fatal("base file regeneration broken")
	}
}

func TestRuntimeCopiedToClientTree(t *testing.T) {
	opts, s := runApp(t)
	for _, name := range []string{"boot.ts", "live.ts", "islands.ts", "actions.ts", "props.ts", "tc.ts"} {
		if _, err := os.Stat(filepath.Join(opts.Root, ".templatic", "runtime", name)); err != nil {
			t.Fatalf("runtime %s not copied: %v", name, err)
		}
	}
	if len(s.RuntimeFiles) == 0 {
		t.Fatal("summary missing runtime files")
	}
}

func TestIslandChunksResolveAgainstManifestURL(t *testing.T) {
	opts, _ := runApp(t)

	// Manifest chunk URLs are relative, so the client must resolve them against the manifest URL.
	islands := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "islands.ts"))
	if !strings.Contains(islands, "new URL(mod.file, baseURL)") {
		t.Fatal("runtime islands.ts must resolve chunk URLs against the manifest base URL")
	}
	boot := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "boot.ts"))
	if !strings.Contains(boot, "document.baseURI") {
		t.Fatal("runtime boot.ts must resolve the manifest URL against the document base")
	}
}

func TestRegionMorphPreservesMarker(t *testing.T) {
	opts, _ := runApp(t)

	// Swap payloads carry the region's inner content; morphing outerHTML would replace the marker.
	live := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "live.ts"))
	if !strings.Contains(live, `morphStyle: "innerHTML"`) {
		t.Fatal("runtime live.ts must morph region children (innerHTML) to preserve the region marker")
	}
}

func TestBootPhasesAreRecorded(t *testing.T) {
	opts, _ := runApp(t)
	boot := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "boot.ts"))
	if !strings.Contains(boot, "tcPhase(") {
		t.Fatal("runtime boot.ts must run boot phases through tcPhase records so degradations are visible in tcState()")
	}
	if !strings.Contains(boot, "console.error") {
		t.Fatal("runtime boot.ts must console.error phase failures — swallowed degradations are the vacuous-stuck class")
	}
}

func TestSwapMorphChainContainsFailures(t *testing.T) {
	opts, _ := runApp(t)
	live := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "live.ts"))
	if !strings.Contains(live, ".catch(") {
		t.Fatal("runtime live.ts must contain morph-chain failures (one rejection must not poison every later swap)")
	}
	if !strings.Contains(live, "tcPhase(\"live\"") {
		t.Fatal("runtime live.ts must record a failed morph in the state record (tcPhase) so it is visible")
	}
	// The morph runtime is boot-tier: every live page can receive a swap.
	if strings.Contains(live, `import("idiomorph")`) {
		t.Fatal("runtime live.ts must import the morph runtime statically (boot-tier), not lazily on first swap")
	}
	if !strings.Contains(live, `from "idiomorph"`) {
		t.Fatal("runtime live.ts must import the morph runtime (`from \"idiomorph\"`)")
	}
}

func TestRegionMorphRemountsIslandHostsAtomically(t *testing.T) {
	opts, _ := runApp(t)
	live := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "live.ts"))
	if !strings.Contains(live, `[data-tc-island]`) {
		t.Fatal("runtime live.ts must scan swapped regions for island hosts")
	}
	if !strings.Contains(live, `document.createComment("tc-island")`) {
		t.Fatal("runtime live.ts must pin island hosts out before morphing so hosts remount as units")
	}
	// The pin must run before the morph call, or the soft-matched host is mangled.
	pinIdx := strings.Index(live, `host.replaceWith(document.createComment("tc-island"))`)
	morphIdx := strings.Index(live, `Idiomorph.morph(region, html`)
	if pinIdx == -1 || morphIdx == -1 || pinIdx > morphIdx {
		t.Fatal("runtime live.ts must pin island hosts out before the morph call")
	}
}

func TestCommandResultsResolvableByClient(t *testing.T) {
	opts, _ := runApp(t)
	commands := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "commands.ts"))
	if !strings.Contains(commands, `envelope.result ?? null`) {
		t.Fatal("runtime commands.ts must resolve the {result} envelope from the dispatch response")
	}
	if !strings.Contains(commands, `send(action: string, args?: unknown, opts?: SendOptions): Promise<unknown>`) {
		t.Fatal("runtime commands.ts send must expose the result payload return type (navHandled opts for the URL layer)")
	}
	if !strings.Contains(commands, `typeof envelope.url === "string"`) {
		t.Fatal("runtime commands.ts must read the {url} declaration off the dispatch envelope")
	}
	if !strings.Contains(commands, `history.pushState(pair, "", target.href)`) {
		t.Fatal("runtime commands.ts must push the server-declared URL with its replay pair")
	}
}

func TestRuntimeCopyAppliesCollectionOps(t *testing.T) {
	opts, _ := runApp(t)
	live := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "live.ts"))
	if !strings.Contains(live, `case "ops":`) || !strings.Contains(live, `case "reset":`) {
		t.Fatal("runtime live.ts must handle the ops and reset event kinds")
	}
	collections := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "collections.ts"))
	if !strings.Contains(collections, "export function applyOps(") {
		t.Fatal("the runtime copy must carry collections.ts")
	}
}

func TestBundleSplitsAndManifests(t *testing.T) {
	opts, s := runApp(t)
	if s.Manifest == "" {
		t.Fatal("bundle did not run")
	}
	if len(s.Bundle) == 0 {
		t.Fatal("bundle produced no outputs")
	}

	mods := readModules(t, opts)
	for _, tag := range []string{"user-card", "big-chart"} {
		url := mods["islands/"+tag].File
		if url == "" {
			t.Fatalf("manifest missing island %s: %#v", tag, mods)
		}
		if _, err := os.Stat(filepath.Join(opts.Root, ".templatic", "dist", filepath.FromSlash(url))); err != nil {
			t.Fatalf("manifest chunk %s not on disk: %v", url, err)
		}
	}

	if len(mods["islands/user-card"].CSS) != 1 {
		t.Fatalf("manifest missing user-card css: %#v", mods["islands/user-card"])
	}
	cssURL := mods["islands/user-card"].CSS[0]
	if _, err := os.Stat(filepath.Join(opts.Root, ".templatic", "dist", filepath.FromSlash(cssURL))); err != nil {
		t.Fatalf("manifest css %s not on disk: %v", cssURL, err)
	}
	if css := read(t, filepath.Join(opts.Root, ".templatic", "dist", filepath.FromSlash(cssURL))); !strings.Contains(css, ".card") {
		t.Fatalf("dist css chunk missing the island's style: %s", css[:min(len(css), 200)])
	}
	if css := mods["islands/big-chart"].CSS; len(css) != 0 {
		t.Fatalf("island without styles must have no css: %q", css)
	}

	islands := read(t, filepath.Join(opts.Root, ".templatic", "runtime", "islands.ts"))
	if !strings.Contains(islands, `mod.css ?? []`) {
		t.Fatal("runtime islands.ts must register the manifest css map")
	}
	if !strings.Contains(islands, `document.createElement("link")`) {
		t.Fatal("runtime islands.ts must inject island stylesheets as <link> elements")
	}

	bigChartURL := mods["islands/big-chart"].File
	if strings.Contains(bigChartURL, "..") {
		t.Fatalf("manifest URL not dist-relative: %q", bigChartURL)
	}
	chunk := read(t, filepath.Join(opts.Root, ".templatic", "dist", filepath.FromSlash(bigChartURL)))
	if strings.Contains(chunk, "treeshake-me-out") {
		t.Fatal("unused export not tree-shaken from island chunk")
	}
	if !strings.Contains(chunk, `console.log("helper"`) {
		t.Fatal("used helper missing from island chunk")
	}
}

func TestEntryLayoutPinned(t *testing.T) {
	opts, _ := runApp(t)

	if _, err := os.Stat(filepath.Join(opts.Root, ".templatic", "dist", "boot.js")); err != nil {
		t.Fatalf("boot.js not at the dist root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(opts.Root, ".templatic", "dist", "tc")); err == nil {
		t.Fatal("dist/tc leaked — entry outputs must not mirror the client tree")
	}
	mods := readModules(t, opts)
	hashed := regexp.MustCompile(`^islands/[a-z0-9-]+-[0-9a-f]{10}\.js$`)
	for _, tag := range []string{"user-card", "big-chart"} {
		url := mods["islands/"+tag].File
		if !hashed.MatchString(url) {
			t.Fatalf("island %s manifest url %q is not content-addressed (islands/{tag}-{hash}.js)", tag, url)
		}
		if _, err := os.Stat(filepath.Join(opts.Root, ".templatic", "dist", filepath.FromSlash(url))); err != nil {
			t.Fatalf("island entry %s not on disk at %s: %v", tag, url, err)
		}
		plain := filepath.Join(opts.Root, ".templatic", "dist", "islands", tag+".js")
		if _, err := os.Stat(plain); err == nil {
			t.Fatalf("un-hashed island entry %s.js survived (stale artifact)", tag)
		}
	}
}

func TestBootIsContentAddressedWithStableShim(t *testing.T) {
	opts, _ := runApp(t)
	dist := filepath.Join(opts.Root, ".templatic", "dist")
	boot := readModules(t, opts)["boot"].File
	if !regexp.MustCompile(`^boot-[0-9a-f]{10}\.js$`).MatchString(boot) {
		t.Fatalf("manifest boot name %q is not content-addressed", boot)
	}
	if _, err := os.Stat(filepath.Join(dist, filepath.FromSlash(boot))); err != nil {
		t.Fatalf("hashed boot bundle missing: %v", err)
	}
	shim := read(t, filepath.Join(dist, "boot.js"))
	if !strings.Contains(shim, boot) || len(shim) > 200 {
		t.Fatalf("boot.js must be a small shim importing the hashed bundle, got %q", shim)
	}
}

func TestSharedChunksDeDuplicate(t *testing.T) {
	opts, _ := runApp(t)
	// The module's implementation string is the signal, not the identifier.
	var definitions int
	found := 0
	err := filepath.Walk(filepath.Join(opts.Root, ".templatic", "dist"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".js") {
			return err
		}
		data := read(t, p)
		if strings.Contains(data, "application/tc-props") {
			definitons := strings.Count(data, "querySelector")
			definitions++
			found += definitons
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if definitions != 1 || found == 0 {
		t.Fatalf("props module defined in %d file(s); want exactly 1 shared chunk", definitions)
	}
}

func TestBindsParsedAndCarried(t *testing.T) {
	_, s := runApp(t)
	for _, isl := range s.Islands {
		if isl.Tag != "big-chart" {
			continue
		}
		if len(isl.Binds) != 2 {
			t.Fatalf("big-chart binds: got %v, want focus+range", isl.Binds)
		}
		want := [][2]string{{"focus", "chart.focus"}, {"range", "chart.range"}}
		for i, b := range isl.Binds {
			if b.Field != want[i][0] || b.Topic != want[i][1] {
				t.Fatalf("bind %d: got %+v, want %v", i, b, want[i])
			}
		}
		return
	}
	t.Fatal("big-chart island not found")
}

func TestBindBaseEmitsTCBindOnlyForBoundLitIslands(t *testing.T) {
	opts, _ := runApp(t)
	litBase := read(t, filepath.Join(opts.Root, ".templatic", "islands", "big-chart.base.ts"))
	if !strings.Contains(litBase, "tcBind(field: string, v: unknown): void") {
		t.Fatal("bound lit island base must emit the tcBind outlet")
	}
	if !strings.Contains(litBase, "{ ...this.props, [field]: v }") {
		t.Fatal("tcBind must assign the whole props object (reactive trip)")
	}
	hooksBase := read(t, filepath.Join(opts.Root, ".templatic", "islands", "user-card.base.ts"))
	if strings.Contains(hooksBase, "tcBind") {
		t.Fatal("unbound hooks island base must not emit tcBind")
	}
}

func TestManifestCarriesBindsMap(t *testing.T) {
	opts, s := runApp(t)
	if s.Manifest == "" {
		t.Fatal("bundle did not run")
	}
	var manifest struct {
		Binds map[string][]struct {
			Topic string `json:"topic"`
			Field string `json:"field"`
		} `json:"binds"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(opts.Root, ".templatic", "dist", "tc.json"))), &manifest); err != nil {
		t.Fatal(err)
	}
	binds, ok := manifest.Binds["big-chart"]
	if !ok || len(binds) != 2 {
		t.Fatalf("manifest binds for big-chart: %#v", manifest.Binds)
	}
	if binds[0].Field != "focus" || binds[0].Topic != "chart.focus" {
		t.Fatalf("bind entry 0: %#v", binds[0])
	}
	if _, hooks := manifest.Binds["user-card"]; hooks {
		t.Fatal("unbound islands must not appear in the binds map")
	}
}

func bindErrorTest(t *testing.T, bindText, wantErr string) {
	t.Helper()
	root := t.TempDir()
	testapp.Copy(t, root)
	chart := filepath.Join(root, "widgets", "chart.templ")
	src := read(t, chart)
	replaced := strings.Replace(src,
		"bind=focus:chart.focus bind=range:chart.range", bindText, 1)
	if replaced == src {
		t.Fatalf("fixture rewrite failed (bind text %q not found)", bindText)
	}
	if err := os.WriteFile(chart, []byte(replaced), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(Options{Root: root})
	if err == nil {
		t.Fatalf("expected generate error containing %q, got none", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("generate error %q missing %q", err.Error(), wantErr)
	}
}

func TestBindFieldMustExistInPropsStruct(t *testing.T) {
	bindErrorTest(t, "bind=nosuchfield:chart.focus",
		"island big-chart: bind=nosuchfield:chart.focus names no field of props struct ChartProps")
}

func TestBindRequiresLitVariant(t *testing.T) {
	root := t.TempDir()
	testapp.Copy(t, root)
	card := filepath.Join(root, "widgets", "card.templ")
	src := read(t, card)
	replaced := strings.Replace(src,
		"//templatic:island tag=user-card props=CardProps",
		"//templatic:island tag=user-card props=CardProps bind=focus:card.focus", 1)
	if replaced == src {
		t.Fatal("fixture rewrite failed (hooks pragma not found)")
	}
	if err := os.WriteFile(card, []byte(replaced), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(Options{Root: root})
	if err == nil || !strings.Contains(err.Error(), "bind= requires variant=lit") {
		t.Fatalf("expected bind-on-hooks error, got %v", err)
	}
}

func TestBindGrammar(t *testing.T) {
	bindErrorTest(t, "bind=focus", "must be field:topic")
	// A second colon lands in the topic half and is rejected by the same rule.
	bindErrorTest(t, "bind=focus:chart!.focus", "must match")
	bindErrorTest(t, "bind=focus:not-a-topic:chart", "must match")
}

func TestDefinesReachTheBuild(t *testing.T) {
	opts := testAppOpts(t)
	opts.Defines = map[string]string{
		"__TC_TEST_FLAG__": `"defined-value"`,
	}
	setIslandScript(t, opts, `onMount((el) => { el.dataset.flag = __TC_TEST_FLAG__; })`)
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	chunk := mustIslandChunk(t, opts, "user-card")
	if !strings.Contains(chunk, "defined-value") {
		t.Fatalf("define not applied to the island chunk: %s", chunk[:min(len(chunk), 300)])
	}
	if strings.Contains(chunk, "__TC_TEST_FLAG__") {
		t.Fatal("define identifier survived in the bundle (it was not replaced)")
	}
}

func TestGoPluginOnLoadAndOnResolve(t *testing.T) {
	opts := testAppOpts(t)
	opts.ESBuildPlugins = []api.Plugin{{
		Name: "tc-test-virtual",
		Setup: func(b api.PluginBuild) {
			b.OnResolve(api.OnResolveOptions{Filter: `^virtual:greeting$`},
				func(api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{Path: "virtual:greeting", Namespace: "tcvirtual"}, nil
				})
			b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "tcvirtual"},
				func(api.OnLoadArgs) (api.OnLoadResult, error) {
					contents := `export const greeting = "hello-from-plugin";`
					return api.OnLoadResult{Contents: &contents, Loader: api.LoaderTS}, nil
				})
		},
	}}
	setIslandScript(t, opts, `import { greeting } from "virtual:greeting";
	onMount((el) => { el.dataset.greeting = greeting; })`)
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	chunk := mustIslandChunk(t, opts, "user-card")
	if !strings.Contains(chunk, "hello-from-plugin") {
		t.Fatalf("plugin-provided virtual module missing from the island chunk: %s", chunk[:min(len(chunk), 300)])
	}
}

func TestFixtureStaysPluginFree(t *testing.T) {
	if _, err := Run(testAppOpts(t)); err != nil {
		t.Fatalf("pristine fixture must build without extensions: %v", err)
	}
}

func TestBuildMutatorRunsLast(t *testing.T) {
	opts := testAppOpts(t)
	opts.Build = func(bo *api.BuildOptions) {
		if !bo.Splitting {
			t.Error("mutator must see templatic's own settings")
		}
		if bo.Plugins == nil && opts.ESBuildPlugins == nil {
		}
		bo.Banner = map[string]string{"js": "/* tc-banner */"}
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	boot := read(t, filepath.Join(opts.Root, ".templatic", "dist", mustBootName(t, opts)))
	if !strings.Contains(boot, "tc-banner") {
		t.Fatal("Build mutator did not reach the build (banner missing)")
	}
}

func mustIslandChunk(t *testing.T, opts Options, tag string) string {
	t.Helper()
	url := readModules(t, opts)["islands/"+tag].File
	if url == "" {
		t.Fatalf("manifest missing island %q", tag)
	}
	return read(t, filepath.Join(opts.Root, ".templatic", "dist", filepath.FromSlash(url)))
}

func mustBootName(t *testing.T, opts Options) string {
	t.Helper()
	boot := readModules(t, opts)["boot"].File
	if boot == "" {
		t.Fatal("manifest has no boot entry")
	}
	return boot
}

func TestIslandConstructorsEmitted(t *testing.T) {
	root, s := runApp(t)
	got := read(t, filepath.Join(root.Root, "widgets", "card_tc.go"))
	for _, want := range []string{
		"// Code generated by templatic generate. DO NOT EDIT.",
		"package widgets",
		`"github.com/nkapatos/templatic/ui"`,
		"func UserCardIsland(props CardProps) templ.Component {",
		`return ui.Island("user-card", props, UserCard(props))`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("card_tc.go missing %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root.Root, "widgets", "plain_tc.go")); !os.IsNotExist(err) {
		t.Fatalf("plain.templ has no island; plain_tc.go must not exist (%v)", err)
	}
	if !slices.Contains(s.Emitted, "widgets/card_tc.go") {
		t.Fatalf("emitted: %v", s.Emitted)
	}
}

func TestIslandConstructorMirrorsParams(t *testing.T) {
	sig, err := parseComponentSig("templ Editor(extra []store.Link, props EditorProps, n, m int)")
	if err != nil {
		t.Fatal(err)
	}
	d, err := islandConstructor(Island{Tag: "x-editor", PropsType: "EditorProps", Component: "Editor"}, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "func EditorIsland(extra []store.Link, props EditorProps, n int, m int) templ.Component") ||
		!strings.Contains(d, `ui.Island("x-editor", props, Editor(extra, props, n, m))`) {
		t.Fatal(d)
	}
	if _, err := islandConstructor(Island{PropsType: "Other", Component: "Editor"}, sig); err == nil {
		t.Fatal("missing props param must fail")
	}
}

func TestGenGoPrunesImportsAndRemovesStale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x_tc.go")
	templGo := []byte("package w\nimport (\n\t\"fmt\"\n\tst \"example/store\"\n)\n")
	if _, _, err := writeGenGo(path, templGo, []string{"func A(l st.Link) templ.Component { return ui.Island(\"a-b\", l, nil) }\n"}); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if strings.Contains(got, `"fmt"`) || !strings.Contains(got, `st "example/store"`) {
		t.Fatal(got)
	}
	if _, _, err := writeGenGo(path, templGo, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale generated file kept")
	}
}

func TestRegionPragmaEmitsDeclarations(t *testing.T) {
	root, _ := runApp(t)
	got := read(t, filepath.Join(root.Root, "widgets", "regions_tc.go"))
	for _, want := range []string{
		`"github.com/nkapatos/templatic/live"`,
		"type ListRegionArgs struct {\n\tItems []string\n\tTitle string\n}",
		`var ListRegionRegion = live.DeclareSessionRegion[ListRegionArgs]("w.list")`,
		"func init() {\n\tListRegionRegion.Renders(func(a ListRegionArgs) templ.Component { return ListRegion(a.Items, a.Title) })\n}",
		"var PanelRegion = live.DeclareSessionRegion[CardProps](PanelTopic)",
		"func init() { PanelRegion.Renders(Panel) }",
		"var EmptyRegion = live.DeclareSessionRegion[struct{}](\"w.empty\")",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("regions_tc.go missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"strings"`) || strings.Contains(got, `"github.com/nkapatos/templatic"`+"\n") {
		t.Fatalf("unused imports kept:\n%s", got)
	}
}

func TestRegionPragmaGrammar(t *testing.T) {
	for _, bad := range []string{
		"//templatic:region",
		"//templatic:region topic=a.b topic-of=X",
		"//templatic:region topic-of=a.b.c",
		"//templatic:region topic=a b",
		"//templatic:region tag=x",
	} {
		if _, _, _, err := parseRegionPragma(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if s, _, ok, err := parseRegionPragma("//templatic:region topic=my.topic"); !ok || err != nil || s.topicExpr != `"my.topic"` {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestBindCheckReadsLiteralsAndConsts(t *testing.T) {
	dir := t.TempDir()
	testapp.WriteModule(t, dir, "example.test/app")
	writeFiles(t, dir, map[string]string{
		"ui/ui.go": "package ui\nconst Sel = \"x.sel\"\n",
		"a/a.go": `package a
import (
	tl "github.com/nkapatos/templatic/live"
	"example.test/app/ui"
)
func r(w *tl.World, id string) {
	tl.NewCell(w, "x.lit", 0)
	tl.NewCell(w, ui.Sel, 0)
	tl.NewCell(w, "x."+id, 0)
}
`})
	prog := mustLoad(t, dir)
	isl := func(topic string) []Island {
		return []Island{{Tag: "x-y", File: "a.templ", Binds: []Bind{{Field: "f", Topic: topic}}}}
	}
	for _, ok := range []string{"x.lit", "x.sel"} {
		if err := checkBinds(prog, isl(ok)); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	err := checkBinds(prog, isl("x.lir"))
	if err == nil || !strings.Contains(err.Error(), `did you mean "x.lit"`) || !strings.Contains(err.Error(), "a/a.go:9") {
		t.Fatalf("missing topic: %v", err)
	}
}

func TestCommandsTSFromDeclarations(t *testing.T) {
	dir := t.TempDir()
	testapp.WriteModule(t, dir, "example.test/app")
	files := map[string]string{
		"ui/ui.go": "package ui\ntype Open struct {\n\tID string `json:\"id\"`\n}\n",
		"todo/todo.go": `package todo
import (
	"github.com/nkapatos/templatic/live"
	"example.test/app/ui"
)
const AddName = "todo.add"
type Add struct {
	Text string ` + "`json:\"text\"`" + `
	Tags []string ` + "`json:\"tags\"`" + `
}
var (
	A = live.NewCommand[Add](AddName)
	O = live.NewCommand[ui.Open]("todo.open")
	R = live.NewCommand[struct{}]("todo.reset")
)
func dyn(n string) { live.NewCommand[Add]("todo." + n) }
`}
	writeFiles(t, dir, files)
	opts := Options{Root: dir, ClientDir: "client"}
	if err := writeCommandsTS(opts, mustLoad(t, dir)); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(dir, ".templatic", "commands.ts"))
	for _, want := range []string{
		`import { send as rawSend } from "templatic";`,
		"export declare namespace todo {\n\tinterface Add {\n\t\ttext: string;\n\t\ttags: string[];\n\t}\n}",
		"export declare namespace ui {\n\tinterface Open {\n\t\tid: string;\n\t}\n}",
		`"todo.add": todo.Add;`,
		`"todo.open": ui.Open;`,
		`"todo.reset": Record<string, never>;`,
		"export function send<K extends CommandName, R = unknown>(name: K, args: Commands[K]): Promise<R | null>",
		"// note: todo/todo.go:16 (computed name)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("commands.ts missing %q:\n%s", want, got)
		}
	}
	writeFiles(t, dir, map[string]string{"todo/todo.go": "package todo\n"})
	if err := writeCommandsTS(opts, mustLoad(t, dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".templatic", "commands.ts")); !os.IsNotExist(err) {
		t.Fatal("stale commands.ts kept")
	}
}

func TestClientDepsChecked(t *testing.T) {
	setVersion := func(t *testing.T, opts Options, pkg, version string) {
		t.Helper()
		p := filepath.Join(opts.Root, "node_modules", pkg, "package.json")
		if err := os.WriteFile(p, []byte(`{"name":"`+pkg+`","version":"`+version+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("in range but not tested", func(t *testing.T) {
		opts := testAppOpts(t)
		setVersion(t, opts, "lit", "3.4.0")
		setVersion(t, opts, "idiomorph", "0.8.5")
		if _, err := Run(opts); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("out of range", func(t *testing.T) {
		opts := testAppOpts(t)
		setVersion(t, opts, "lit", "4.0.0")
		setVersion(t, opts, "idiomorph", "0.9.0")
		_, err := Run(opts)
		for _, want := range []string{
			"lit 4.0.0 is installed (",
			"), templatic needs ^3.3.3 (tested with 3.3.3)",
			"idiomorph 0.9.0 is installed (",
			"), templatic needs ~0.8.0 (tested with 0.8.0)",
			"run: npm install --save-exact idiomorph@0.8.0 lit@3.3.3",
		} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want %q, got %v", want, err)
			}
		}
	})

	t.Run("unparseable version", func(t *testing.T) {
		opts := testAppOpts(t)
		setVersion(t, opts, "lit", "latest")
		_, err := Run(opts)
		if err == nil || !strings.Contains(err.Error(), "lit latest is installed") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		opts := testAppOpts(t)
		if err := os.RemoveAll(filepath.Join(opts.Root, "node_modules", "lit")); err != nil {
			t.Fatal(err)
		}
		_, err := Run(opts)
		for _, want := range []string{
			"lit is not installed (templatic needs ^3.3.3, tested with 3.3.3)",
			"run: npm install --save-exact lit@3.3.3",
		} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want %q, got %v", want, err)
			}
		}
	})
}

func TestSatisfies(t *testing.T) {
	cases := []struct {
		rng, version string
		want         bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"^1.2.3", "1.2.3", true},
		{"^1.2.3", "1.9.0", true},
		{"^1.2.3", "1.2.2", false},
		{"^1.2.3", "2.0.0", false},
		{"^1.2.3", "0.9.0", false},
		{"^0.2.3", "0.2.3", true},
		{"^0.2.3", "0.2.9", true},
		{"^0.2.3", "0.3.0", false},
		{"^0.2.3", "0.2.2", false},
		{"^0.0.3", "0.0.3", true},
		{"^0.0.3", "0.0.4", false},
		{"^0.0.3", "0.0.2", false},
		{"~1.2.3", "1.2.3", true},
		{"~1.2.3", "1.2.9", true},
		{"~1.2.3", "1.3.0", false},
		{"~1.2.3", "1.2.2", false},
		{"~0.8.0", "0.8.5", true},
		{"~0.8.0", "0.9.0", false},
		{"^3.3.3", "3.3.3-beta.1", false},
		{"^3.3.3", "3.4.0-rc.1", false},
		{"1.2.3", "1.2.3-beta.1", false},
		{"^3.3.3", "3.4.0+build.5", true},
		{"^3.3.3", "3.3.2+build.5", false},
		{"^3.3.3", "latest", false},
		{"^3.3.3", "", false},
	}
	for _, c := range cases {
		got, err := satisfies(c.rng, c.version)
		if err != nil {
			t.Fatalf("satisfies(%q, %q): %v", c.rng, c.version, err)
		}
		if got != c.want {
			t.Errorf("satisfies(%q, %q) = %v, want %v", c.rng, c.version, got, c.want)
		}
	}
	for _, rng := range []string{"", "*", "1.x", ">=1.2.3", "^1.2", "~1", "1.2.3 - 2.0.0", "^1.2.3 || ^2.0.0", "latest", "^1.2.3-beta.1", "^01.2.3"} {
		if _, err := satisfies(rng, "1.2.3"); err == nil || !strings.Contains(err.Error(), "templatic bug") {
			t.Errorf("satisfies(%q): want unsupported-range error, got %v", rng, err)
		}
	}
}

func TestClientDepsAliasSkipsCheck(t *testing.T) {
	opts := testAppOpts(t)
	if err := os.WriteFile(filepath.Join(opts.Root, "node_modules", "lit", "package.json"), []byte(`{"name":"lit","version":"9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	opts.Alias = map[string]string{"lit": "./node_modules/lit/index.js"}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
}

func TestTakeoverLeavesDataScriptsInTheMarkup(t *testing.T) {
	opts := testAppOpts(t)
	opts.SkipBundle = true
	path := filepath.Join(opts.Root, "widgets", "card.templ")
	src := strings.Replace(read(t, path), "\t<style>", "\t<script type=\"application/json\" id=\"card-data\">{\"seed\":41}</script>\n\t<style>", 1)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if goFile := read(t, filepath.Join(opts.Root, "widgets", "card_templ.go")); !strings.Contains(goFile, "application/json") {
		t.Fatal("data script dropped from the markup")
	}
	gen, _ := filepath.Glob(filepath.Join(opts.Root, ".templatic", "islands", "user-card.*"))
	if len(gen) == 0 {
		t.Fatal("no generated island files")
	}
	for _, f := range gen {
		if strings.Contains(read(t, f), "seed") {
			t.Fatalf("data script consumed as island TS (%s)", filepath.Base(f))
		}
	}
	if !strings.Contains(read(t, filepath.Join(opts.Root, ".templatic", "islands", "user-card.hooks.ts")), "mounted") {
		t.Fatal("the island's own script is lost")
	}
}

func TestOrphanedGeneratedGoIsRemoved(t *testing.T) {
	opts := testAppOpts(t)
	opts.SkipBundle = true
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	w := filepath.Join(opts.Root, "widgets")
	hand := filepath.Join(w, "gone_tc.go")
	if err := os.WriteFile(hand, []byte("package widgets\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(w, "card.templ")); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"card_templ.go", "card_tc.go"} {
		if _, err := os.Stat(filepath.Join(w, f)); !os.IsNotExist(err) {
			t.Errorf("%s survived its .templ: %v", f, err)
		}
	}
	if _, err := os.Stat(hand); err != nil {
		t.Fatalf("hand-written file removed: %v", err)
	}
}

func TestStubsWithoutIslandsDir(t *testing.T) {
	opts := testAppOpts(t)
	if err := os.RemoveAll(filepath.Join(opts.Root, "client", "islands")); err != nil {
		t.Fatal(err)
	}
	opts.SkipBundle = true
	s, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, isl := range s.Islands {
		if _, err := os.Stat(filepath.Join(opts.Root, "client", "islands", isl.Tag+".ts")); err != nil {
			t.Error(err)
		}
	}
}
