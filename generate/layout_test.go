package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedOutputsLiveInGenDir(t *testing.T) {
	opts, s := runApp(t)
	gen := filepath.Join(opts.Root, GenDir)
	for _, rel := range []string{
		".templatic", "runtime/boot.ts", "runtime/index.ts", "tsconfig.json",
		"islands/user-card.types.ts", "islands/user-card.hooks.ts", "islands/user-card.base.ts",
		"islands/user-card.css", "dist/tc.json", "dist/boot.js", "dist/.generations.json",
	} {
		if !exists(filepath.Join(gen, filepath.FromSlash(rel))) {
			t.Errorf("%s/%s not written", GenDir, rel)
		}
	}
	ents, err := os.ReadDir(filepath.Join(opts.Root, "client", "islands"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		n := e.Name()
		if strings.Contains(n, ".base.") || strings.Contains(n, ".types.") || strings.Contains(n, ".hooks.") || strings.HasSuffix(n, ".css") {
			t.Errorf("generated file %s written into client/islands", n)
		}
	}
	for _, rel := range []string{"client/tc", "client/tsconfig.templatic.json", "client/gen", "static"} {
		if exists(filepath.Join(opts.Root, rel)) {
			t.Errorf("%s written outside %s", rel, GenDir)
		}
	}
	if s.Manifest != ".templatic/dist/tc.json" {
		t.Errorf("Manifest = %q", s.Manifest)
	}
	if got := read(t, filepath.Join(gen, "tsconfig.json")); !strings.Contains(got, `"templatic/islands/*": ["./islands/*"]`) || !strings.Contains(got, `"templatic": ["./runtime/index.ts"]`) {
		t.Errorf("tsconfig paths wrong:\n%s", got)
	}
	define := read(t, filepath.Join(opts.Root, "client", "islands", "user-card.ts"))
	if !strings.Contains(define, `from "templatic/islands/user-card.base"`) {
		t.Errorf("define file does not import the generated base by specifier:\n%s", define)
	}
	if !strings.Contains(read(t, filepath.Join(gen, ".templatic")), `"version"`) {
		t.Error("marker has no version")
	}
}

func TestRelativeScriptImportsRebased(t *testing.T) {
	opts, _ := runApp(t)
	hooks := read(t, filepath.Join(opts.Root, GenDir, "islands", "big-chart.hooks.ts"))
	if !strings.Contains(hooks, `"../../client/islands/chart-util"`) {
		t.Fatalf("relative import not rebased:\n%s", hooks)
	}
}

func TestUserFilesInClientDirSurvive(t *testing.T) {
	opts, _ := runApp(t)
	define := filepath.Join(opts.Root, "client", "islands", "user-card.ts")
	css := filepath.Join(opts.Root, "client", "islands", "user-card.css")
	other := filepath.Join(opts.Root, "client", "islands", "mine.css")
	writeFile(t, define, "// mine\n")
	writeFile(t, css, "/* user css */\n")
	writeFile(t, other, "/* other */\n")
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{define: "// mine\n", css: "/* user css */\n", other: "/* other */\n"} {
		if got := read(t, p); got != want {
			t.Errorf("%s changed: %q", p, got)
		}
	}
}

func TestDistDirValidation(t *testing.T) {
	cases := []struct {
		name  string
		dist  string
		setup func(root string)
		want  string
	}{
		{"root", ".", nil, "module root"},
		{"root slash", "./", nil, "module root"},
		{"absolute", "/tmp/x", nil, "relative path inside the module root"},
		{"escapes", "../out", nil, "relative path inside the module root"},
		{"gen dir", ".templatic", nil, "must not be .templatic itself"},
		{"inside gen dir", ".templatic/runtime", nil, "holds generated sources"},
		{"client dir", "client", nil, "equals or contains ClientDir"},
		{"contains client", ".", nil, "module root"},
		{"holds templ", "widgets", nil, "holds source file"},
		{"holds go", "out", func(r string) { writeFile(t, filepath.Join(r, "out", "x", "a.go"), "package x\n") }, "holds source file"},
		{"non-empty foreign", "out", func(r string) { writeFile(t, filepath.Join(r, "out", "mine.txt"), "x") },
			"out is not empty and was not created by templatic: empty it or choose another DistDir"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts := testAppOpts(t)
			opts.DistDir = c.dist
			if c.setup != nil {
				c.setup(opts.Root)
			}
			_, err := Run(opts)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if exists(filepath.Join(opts.Root, GenDir)) {
				t.Errorf("%s written despite the refused DistDir", GenDir)
			}
			if exists(filepath.Join(opts.Root, "widgets", "card_templ.go")) {
				t.Error("Go glue written despite the refused DistDir")
			}
		})
	}
}

func TestDistDirContainingClientDirRefused(t *testing.T) {
	opts := testAppOpts(t)
	opts.ClientDir = "web/client"
	opts.DistDir = "web"
	_, err := Run(opts)
	if err == nil || !strings.Contains(err.Error(), "equals or contains ClientDir") {
		t.Fatalf("err = %v", err)
	}
}

func TestDistDirCleaned(t *testing.T) {
	opts := testAppOpts(t)
	opts.DistDir = "./out/dist/"
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(opts.Root, "out", "dist", "tc.json")) {
		t.Fatal("tc.json not under the cleaned DistDir")
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	m := read(t, filepath.Join(opts.Root, "out", "dist", "tc.json"))
	if strings.Contains(m, "out/dist//") {
		t.Fatalf("hashing used an unclean path:\n%s", m)
	}
}

func TestDistDirEmptyExistingAccepted(t *testing.T) {
	opts := testAppOpts(t)
	opts.DistDir = "out"
	if err := os.MkdirAll(filepath.Join(opts.Root, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
}

func TestGenDirWithoutMarkerRefused(t *testing.T) {
	opts := testAppOpts(t)
	writeFile(t, filepath.Join(opts.Root, GenDir, "mine.txt"), "x")
	if _, err := Run(opts); err == nil || !strings.Contains(err.Error(), "was not created by templatic") {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultAssetsBase(t *testing.T) {
	opts, _ := runApp(t)
	if m := read(t, filepath.Join(opts.Root, GenDir, "dist", "tc.json")); !strings.Contains(m, `"base": "/_templatic/"`) {
		t.Fatalf("default base wrong:\n%s", m)
	}
}

func gens(t *testing.T, dist string) [][]string {
	t.Helper()
	g, err := readGenerations(dist)
	if err != nil {
		t.Fatal(err)
	}
	return g.Generations
}

func TestPruneKeepsCurrentAndPreviousGeneration(t *testing.T) {
	dist := t.TempDir()
	touch := func(names ...string) {
		for _, n := range names {
			writeFile(t, filepath.Join(dist, n), n)
		}
	}
	unknown := filepath.Join(dist, "robots.txt")
	writeFile(t, unknown, "keep me")

	touch("tc.json", "a.js", "common.js")
	if err := recordGeneration(dist, []string{"tc.json", "a.js", "common.js"}); err != nil {
		t.Fatal(err)
	}
	touch("b.js")
	if err := recordGeneration(dist, []string{"tc.json", "b.js", "common.js"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a.js", "b.js", "common.js"} {
		if !exists(filepath.Join(dist, n)) {
			t.Fatalf("%s pruned after two generations", n)
		}
	}
	touch("c.js")
	if err := recordGeneration(dist, []string{"tc.json", "c.js", "common.js"}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dist, "a.js")) {
		t.Error("a.js (only in the oldest generation) survived")
	}
	for _, n := range []string{"b.js", "c.js", "common.js", "tc.json", "robots.txt"} {
		if !exists(filepath.Join(dist, n)) {
			t.Errorf("%s was pruned", n)
		}
	}
	if got := len(gens(t, dist)); got != 2 {
		t.Errorf("record holds %d generations, want 2", got)
	}
}

func TestPruneNeverDeletesUnrecordedFiles(t *testing.T) {
	dist := t.TempDir()
	writeFile(t, filepath.Join(dist, "chunks", "mine.js"), "x")
	if err := recordGeneration(dist, []string{"a.js"}); err != nil {
		t.Fatal(err)
	}
	if err := recordGeneration(dist, []string{"b.js"}); err != nil {
		t.Fatal(err)
	}
	if err := recordGeneration(dist, []string{"c.js"}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dist, "chunks", "mine.js")) {
		t.Fatal("unrecorded file deleted")
	}
}

func TestPruneIgnoresPathsEscapingDist(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	victim := filepath.Join(root, "victim.txt")
	writeFile(t, victim, "x")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGen := func(g [][]string) {
		if err := writeGenerations(dist, generations{g}); err != nil {
			t.Fatal(err)
		}
	}
	writeGen([][]string{{"../victim.txt"}, {"x"}})
	if err := recordGeneration(dist, []string{"y"}); err != nil {
		t.Fatal(err)
	}
	if !exists(victim) {
		t.Fatal("a recorded path outside dist was deleted")
	}
}

func TestBuildPrunesOnlyOlderGenerations(t *testing.T) {
	opts, _ := runApp(t)
	dist := filepath.Join(opts.Root, GenDir, "dist")
	user := filepath.Join(dist, "robots.txt")
	writeFile(t, user, "keep")
	writeFile(t, filepath.Join(dist, "chunks", "chunk-OLD.js"), "old")
	writeFile(t, filepath.Join(dist, "chunks", "chunk-PREV.js"), "prev")
	if err := writeGenerations(dist, generations{[][]string{{"chunks/chunk-OLD.js"}, {"chunks/chunk-PREV.js"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Root: opts.Root}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dist, "chunks", "chunk-OLD.js")) {
		t.Error("chunk from an older generation survived")
	}
	if !exists(filepath.Join(dist, "chunks", "chunk-PREV.js")) {
		t.Error("previous generation's chunk was pruned")
	}
	if !exists(user) {
		t.Error("unrecorded file was pruned")
	}
	if !exists(filepath.Join(dist, "tc.json")) || !exists(filepath.Join(dist, "boot.js")) {
		t.Error("declared output pruned")
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tc.json")
	if err := writeAtomic(p, []byte("one")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(p, []byte("one")); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if !st.ModTime().Equal(old.Truncate(time.Second)) && st.ModTime().After(old.Add(time.Minute)) {
		t.Error("unchanged content was rewritten")
	}
	if err := writeAtomic(p, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); got != "two" {
		t.Fatalf("content %q", got)
	}
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	if !reflect.DeepEqual(names, []string{"tc.json"}) {
		t.Fatalf("temp files left behind: %v", names)
	}
}

func TestManifestWrittenByRename(t *testing.T) {
	opts, _ := runApp(t)
	dist := filepath.Join(opts.Root, GenDir, "dist")
	ents, _ := os.ReadDir(dist)
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file %s left in dist", e.Name())
		}
	}
	p := filepath.Join(dist, "tc.json")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.ModTime().After(old.Add(time.Minute)) {
		t.Error("identical tc.json was rewritten")
	}
}
