package generate

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

type fnPlugin struct {
	name  string
	setup func(*Builder) error
}

func (p fnPlugin) Name() string           { return p.name }
func (p fnPlugin) Setup(b *Builder) error { return p.setup(b) }

func TestPluginsRunInRegistrationOrder(t *testing.T) {
	opts := testAppOpts(t)
	var order []string
	mk := func(name string) Plugin {
		return fnPlugin{name, func(b *Builder) error {
			order = append(order, name+":setup")
			b.OnConfig(func(*Options) error { order = append(order, name+":config"); return nil })
			b.OnScan(func(*Summary) error { order = append(order, name+":scan"); return nil })
			b.OnEntries(func(*Entries) error { order = append(order, name+":entries"); return nil })
			b.OnBuildOptions(func(*api.BuildOptions) error { order = append(order, name+":build"); return nil })
			b.OnManifest(func(*Manifest) error { order = append(order, name+":manifest"); return nil })
			b.OnDone(func(*Result) error { order = append(order, name+":done"); return nil })
			return nil
		}}
	}
	opts.Plugins = []Plugin{mk("a"), mk("b")}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(order, " ")
	want := "a:setup b:setup a:config b:config a:scan b:scan a:entries b:entries a:build b:build a:manifest b:manifest a:done b:done"
	if got != want {
		t.Fatalf("hook order\n got: %s\nwant: %s", got, want)
	}
}

func TestPluginErrorsAreAttributed(t *testing.T) {
	boom := errors.New("boom")
	hooks := map[string]func(b *Builder, err error){
		"config":  func(b *Builder, err error) { b.OnConfig(func(*Options) error { return err }) },
		"scan":    func(b *Builder, err error) { b.OnScan(func(*Summary) error { return err }) },
		"entries": func(b *Builder, err error) { b.OnEntries(func(*Entries) error { return err }) },
		"build":   func(b *Builder, err error) { b.OnBuildOptions(func(*api.BuildOptions) error { return err }) },
		"mfst":    func(b *Builder, err error) { b.OnManifest(func(*Manifest) error { return err }) },
		"done":    func(b *Builder, err error) { b.OnDone(func(*Result) error { return err }) },
	}
	for name, reg := range hooks {
		t.Run(name, func(t *testing.T) {
			opts := testAppOpts(t)
			opts.Plugins = []Plugin{
				fnPlugin{"fine", func(*Builder) error { return nil }},
				fnPlugin{"bad-" + name, func(b *Builder) error { reg(b, boom); return nil }},
			}
			_, err := Run(opts)
			if err == nil || !errors.Is(err, boom) {
				t.Fatalf("want wrapped boom, got %v", err)
			}
			if !strings.HasPrefix(err.Error(), "plugin bad-"+name+": ") {
				t.Fatalf("error not attributed: %v", err)
			}
		})
	}
	t.Run("setup", func(t *testing.T) {
		opts := testAppOpts(t)
		opts.Plugins = []Plugin{fnPlugin{"nosetup", func(*Builder) error { return boom }}}
		if _, err := Run(opts); err == nil || !strings.HasPrefix(err.Error(), "plugin nosetup: ") {
			t.Fatalf("setup error not attributed: %v", err)
		}
	})
}

func TestOnConfigAdjustsOptions(t *testing.T) {
	opts := testAppOpts(t)
	opts.Plugins = []Plugin{fnPlugin{"cfg", func(b *Builder) error {
		b.OnConfig(func(o *Options) error {
			o.Defines = map[string]string{"__TC_TEST_FLAG__": "true"}
			return nil
		})
		return nil
	}}}
	setIslandScript(t, opts, `onMount((el) => { el.dataset.flag = String(__TC_TEST_FLAG__); })`)
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if chunk := mustIslandChunk(t, opts, "user-card"); strings.Contains(chunk, "__TC_TEST_FLAG__") {
		t.Fatal("OnConfig define did not reach the build")
	}
}

func TestOnScanSeesIslandsAndWatch(t *testing.T) {
	opts := testAppOpts(t)
	var tags []string
	opts.Plugins = []Plugin{fnPlugin{"scan", func(b *Builder) error {
		b.Watch("public/**", "extra/*.txt")
		b.OnScan(func(s *Summary) error {
			for _, isl := range s.Islands {
				tags = append(tags, isl.Tag)
			}
			if len(s.Emitted) == 0 || len(s.RuntimeFiles) == 0 {
				t.Error("OnScan runs after emit and runtime copy")
			}
			return nil
		})
		return nil
	}}}
	s, err := Run(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) == 0 || len(tags) != len(s.Islands) {
		t.Fatalf("OnScan saw %v, run found %d islands", tags, len(s.Islands))
	}
	if strings.Join(s.Watch, ",") != "public/**,extra/*.txt" {
		t.Fatalf("watch globs not recorded: %v", s.Watch)
	}
}

func TestOnScanRunsWithSkipBundle(t *testing.T) {
	opts := testAppOpts(t)
	opts.SkipBundle = true
	seen := false
	opts.Plugins = []Plugin{fnPlugin{"scan", func(b *Builder) error {
		b.OnScan(func(*Summary) error { seen = true; return nil })
		b.OnManifest(func(*Manifest) error { t.Error("OnManifest must not run without a bundle"); return nil })
		return nil
	}}}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("OnScan did not run")
	}
}

func TestOnEntriesAddsHashedWorkerEntry(t *testing.T) {
	opts := testAppOpts(t)
	if err := os.MkdirAll(filepath.Join(opts.Root, "client", "workers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.Root, "client", "workers", "sync.ts"),
		[]byte("self.onmessage = (e) => { (self as any).postMessage('worker-says:' + e.data); };\nexport {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts.Plugins = []Plugin{fnPlugin{"worker", func(b *Builder) error {
		b.OnEntries(func(e *Entries) error {
			if e.List[0].Kind != EntryBoot {
				t.Errorf("boot entry must come first: %+v", e.List[0])
			}
			e.Add("workers/sync", "client/workers/sync.ts")
			return nil
		})
		return nil
	}}}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	url := readModules(t, opts)["workers/sync"].File
	if !regexp.MustCompile(`^workers/sync-[0-9a-f]{10}\.js$`).MatchString(url) {
		t.Fatalf("worker entry not hashed in the manifest: %q", url)
	}
	if !strings.Contains(read(t, filepath.Join(opts.Root, ".templatic", "dist", filepath.FromSlash(url))), "worker-says:") {
		t.Fatal("hashed worker output missing its code")
	}
	if _, err := os.Stat(filepath.Join(opts.Root, ".templatic", "dist", "workers", "sync.js")); err == nil {
		t.Fatal("unhashed worker output must not linger")
	}
}

func TestOnEntriesWithCSSHashesJSAndCSS(t *testing.T) {
	for i := 0; i < 8; i++ {
		opts := testAppOpts(t)
		dir := filepath.Join(opts.Root, "client", "pages")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "charts.css"), []byte(".chart{color:red}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "charts.ts"), []byte("import './charts.css';\nconsole.log('charts-entry');\nexport {};\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		opts.Plugins = []Plugin{fnPlugin{"pages", func(b *Builder) error {
			b.OnEntries(func(e *Entries) error { e.Add("modules/charts", "client/pages/charts.ts"); return nil })
			return nil
		}}}
		if _, err := Run(opts); err != nil {
			t.Fatal(err)
		}
		mod := readModules(t, opts)["modules/charts"]
		js, css := mod.File, ""
		if len(mod.CSS) == 1 {
			css = mod.CSS[0]
		}
		if !regexp.MustCompile(`^modules/charts-[0-9a-f]{10}\.js$`).MatchString(js) {
			t.Fatalf("run %d: entry js = %q, want hashed .js", i, js)
		}
		if !regexp.MustCompile(`^modules/charts-[0-9a-f]{10}\.css$`).MatchString(css) {
			t.Fatalf("run %d: entry css = %q, want hashed .css", i, css)
		}
		if !strings.Contains(read(t, filepath.Join(opts.Root, ".templatic", "dist", filepath.FromSlash(js))), "charts-entry") {
			t.Fatalf("run %d: %s is not the js output", i, js)
		}
	}
}

func TestEntriesValidated(t *testing.T) {
	for _, name := range []string{"boot", "islands/x", "../evil", "dup"} {
		opts := testAppOpts(t)
		opts.Plugins = []Plugin{fnPlugin{"bad", func(b *Builder) error {
			b.OnEntries(func(e *Entries) error {
				e.Add(name, ".templatic/runtime/boot.ts")
				if name == "dup" {
					e.Add(name, ".templatic/runtime/boot.ts")
				}
				return nil
			})
			return nil
		}}}
		if _, err := Run(opts); err == nil {
			t.Fatalf("entry name %q must be rejected", name)
		}
	}
}

func TestOnManifestEditsPersistIntoTCJSON(t *testing.T) {
	opts := testAppOpts(t)
	var sawOutputs int
	opts.Plugins = []Plugin{fnPlugin{"meta", func(b *Builder) error {
		b.OnManifest(func(m *Manifest) error {
			sawOutputs = len(m.Outputs)
			if m.Dist == "" || m.Modules["boot"].File == "" || len(m.Modules) < 2 {
				t.Errorf("manifest incomplete at OnManifest: %+v", m)
			}
			m.Meta = map[string]any{"meta": map[string]any{"stamp": "persisted"}}
			return nil
		})
		return nil
	}}}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if sawOutputs == 0 {
		t.Fatal("Manifest.Outputs empty")
	}
	if !strings.Contains(read(t, filepath.Join(opts.Root, ".templatic", "dist", "tc.json")), `"stamp": "persisted"`) {
		t.Fatal("OnManifest edit did not persist into tc.json")
	}
}

func TestOnDoneSeesFinalOutputsAndSurvivesInDist(t *testing.T) {
	opts := testAppOpts(t)
	opts.Plugins = []Plugin{fnPlugin{"done", func(b *Builder) error {
		b.OnDone(func(r *Result) error {
			if _, err := os.Stat(filepath.Join(r.Dist, "tc.json")); err != nil {
				t.Errorf("tc.json must be written before OnDone: %v", err)
			}
			boot := ""
			for _, o := range r.Outputs {
				if _, err := os.Stat(filepath.Join(r.Root, filepath.FromSlash(o.Path))); err != nil {
					t.Errorf("Result output %s is not a file on disk (hashed path expected): %v", o.Path, err)
				}
				if o.Entry == filepath.ToSlash(filepath.Join(".templatic", "runtime", "boot.ts")) {
					boot = o.Path
				}
			}
			if boot == "" || !strings.Contains(boot, "boot-") {
				t.Errorf("boot output missing or unhashed: %q", boot)
			}
			return os.WriteFile(filepath.Join(r.Dist, "report.txt"), []byte("ok"), 0o644)
		})
		return nil
	}}}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(opts.Root, ".templatic", "dist", "report.txt")) != "ok" {
		t.Fatal("file written in OnDone missing")
	}
}

func TestESBuildAdapterAndBuilderPluginOrder(t *testing.T) {
	opts := testAppOpts(t)
	native := func(name string) api.Plugin { return api.Plugin{Name: name, Setup: func(api.PluginBuild) {}} }
	opts.ESBuildPlugins = []api.Plugin{native("legacy")}
	opts.Plugins = []Plugin{ESBuild(native("adapted"))}
	var names []string
	opts.Build = func(o *api.BuildOptions) {
		for _, p := range o.Plugins {
			names = append(names, p.Name)
		}
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "templatic-asset-suffix,legacy,adapted" {
		t.Fatalf("esbuild plugin order: %v", names)
	}
}

func TestBuildOptionsHookRunsBeforeBuildMutator(t *testing.T) {
	opts := testAppOpts(t)
	opts.Plugins = []Plugin{fnPlugin{"bo", func(b *Builder) error {
		b.OnBuildOptions(func(o *api.BuildOptions) error { o.Banner = map[string]string{"js": "/* from-plugin */"}; return nil })
		return nil
	}}}
	opts.Build = func(o *api.BuildOptions) { o.Banner = map[string]string{"js": "/* from-mutator */"} }
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	boot := read(t, filepath.Join(opts.Root, ".templatic", "dist", mustBootName(t, opts)))
	if !strings.Contains(boot, "from-mutator") || strings.Contains(boot, "from-plugin") {
		t.Fatal("Options.Build must run after OnBuildOptions and win")
	}
}

func TestOnEntriesPlainCSSEntry(t *testing.T) {
	opts := testAppOpts(t)
	if err := os.WriteFile(filepath.Join(opts.Root, "client", "app.css"), []byte(".site{color:teal}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts.Plugins = []Plugin{fnPlugin{"css", func(b *Builder) error {
		b.OnEntries(func(e *Entries) error { e.Add("app", "client/app.css"); return nil })
		return nil
	}}}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	mod := readModules(t, opts)["app"]
	if mod.File != "" || len(mod.CSS) != 1 {
		t.Fatalf("modules.app = %+v, want one css and no js", mod)
	}
	css := mod.CSS[0]
	if !regexp.MustCompile(`^app-[0-9a-f]{10}\.css$`).MatchString(css) {
		t.Fatalf("entry app = %q, want hashed .css", css)
	}
	if !strings.Contains(read(t, filepath.Join(opts.Root, ".templatic", "dist", css)), "teal") {
		t.Fatal("stylesheet content missing")
	}
}

type ptrPlugin struct{ name string }

func (p *ptrPlugin) Name() string         { return p.name }
func (p *ptrPlugin) Setup(*Builder) error { return nil }

func TestNilPluginsAreSkipped(t *testing.T) {
	opts := testAppOpts(t)
	opts.SkipBundle = true
	var typed *ptrPlugin
	opts.Plugins = []Plugin{nil, typed}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
}
