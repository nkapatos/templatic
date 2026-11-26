package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type importFixture struct{ dir string }

func newImportFixture(t *testing.T) importFixture {
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOWORK", "off")
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", "github.com/a-h/templ")
	cmd.Dir = root
	ver, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	goSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	w := func(rel, src string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dep := func(mod, pkg, typ string) {
		sub := strings.ReplaceAll(strings.ReplaceAll(mod, "/", "_"), ":", "_")
		w("deps/"+sub+"/go.mod", "module "+mod+"\n\ngo 1.21\n")
		w("deps/"+sub+"/x.go", "package "+pkg+"\n\ntype "+typ+" struct{}\n")
	}
	dep("example.com/foo/v2", "foo", "Foo")
	dep("gopkg.in/x.v3", "x", "X")
	dep("github.com/acme/go-chi", "chi", "C")
	dep("example.com/odd", "weird", "W")
	dep("example.com/ui", "ui", "U")
	w("go.mod", `module example.com/app

go 1.21

require (
	github.com/a-h/templ `+strings.TrimSpace(string(ver))+`
	github.com/nkapatos/templatic v0.0.0
	example.com/foo/v2 v2.0.0
	gopkg.in/x.v3 v3.0.0
	github.com/acme/go-chi v0.0.0
	example.com/odd v0.0.0
	example.com/ui v0.0.0
)

replace github.com/nkapatos/templatic => `+root+`
replace example.com/foo/v2 => ./deps/example.com_foo_v2
replace gopkg.in/x.v3 => ./deps/gopkg.in_x.v3
replace github.com/acme/go-chi => ./deps/github.com_acme_go-chi
replace example.com/odd => ./deps/example.com_odd
replace example.com/ui => ./deps/example.com_ui
`)
	w("go.sum", string(goSum))
	w("views/props.go", `package views

type Props struct{}
`)
	return importFixture{dir}
}

func (f importFixture) generate(t *testing.T, file, imports, sig string) string {
	t.Helper()
	s, err := parseComponentSig(sig)
	if err != nil {
		t.Fatal(err)
	}
	d, err := islandConstructor(Island{Tag: "x-" + strings.ToLower(s.name), PropsType: "Props", Component: s.name}, s)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.dir, "views", file+"_tc.go")
	if _, _, err := writeGenGo(path, []byte("package views\n\nimport (\n"+imports+")\n"), []string{d}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (f importFixture) component(t *testing.T, file, imports, sig string) {
	t.Helper()
	src := "package views\n\nimport (\n\t\"context\"\n\t\"io\"\n\t\"github.com/a-h/templ\"\n" + imports + ")\n\n" +
		"func " + sig + " templ.Component {\n\treturn templ.ComponentFunc(func(context.Context, io.Writer) error { return nil })\n}\n"
	if err := os.WriteFile(filepath.Join(f.dir, "views", file+".go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f importFixture) build(t *testing.T) {
	t.Helper()
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = f.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Go does not compile: %v\n%s", err, out)
	}
}

func TestGenGoResolvesPackageNamesThatDifferFromThePath(t *testing.T) {
	f := newImportFixture(t)
	imports := "\t\"example.com/foo/v2\"\n\t\"gopkg.in/x.v3\"\n\t\"github.com/acme/go-chi\"\n\t\"example.com/odd\"\n\t\"strings\"\n"
	sig := "Card(p Props, a foo.Foo, b x.X, c chi.C, d weird.W)"
	f.component(t, "card", imports[:strings.Index(imports, "\t\"strings\"")], sig)
	got := f.generate(t, "card", imports, "templ "+sig)
	for _, want := range []string{`"example.com/foo/v2"`, `"gopkg.in/x.v3"`, `"github.com/acme/go-chi"`, `"example.com/odd"`} {
		if !strings.Contains(got, want) {
			t.Errorf("import %s dropped:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"strings"`) {
		t.Errorf("unused import kept:\n%s", got)
	}
	f.build(t)

	again := f.generate(t, "card", imports, "templ "+sig)
	if again != got {
		t.Fatalf("output is not deterministic:\n%s\n---\n%s", got, again)
	}
}

func TestGenGoAliasesGeneratedImportsTheUserUses(t *testing.T) {
	f := newImportFixture(t)
	imports := "\t\"example.com/ui\"\n"
	sig := "Box(p Props, u ui.U, live string, templ int)"
	hand := "Box(p Props, u ui.U, a string, b int)"
	f.component(t, "box", imports, hand)
	got := f.generate(t, "box", imports, "templ "+sig)
	for _, want := range []string{`tcui "github.com/nkapatos/templatic/ui"`, `tctempl "github.com/a-h/templ"`, "tcui.Island(", "u ui.U", ") tctempl.Component", `"example.com/ui"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	f.build(t)
}

func TestGenGoKeepsUnaliasedImportsWithoutCollision(t *testing.T) {
	f := newImportFixture(t)
	got := f.generate(t, "plain", "\t\"example.com/foo/v2\"\n", "templ Plain(p Props, a foo.Foo)")
	if strings.Contains(got, "tcui") || strings.Contains(got, "tctempl") || !strings.Contains(got, "ui.Island(") {
		t.Fatalf("no alias expected:\n%s", got)
	}
}

func TestIslandConstructorForwardsVariadicParams(t *testing.T) {
	f := newImportFixture(t)
	sig := "Rows(p Props, first string, rest ...int)"
	f.component(t, "rows", "", sig)
	got := f.generate(t, "rows", "", "templ "+sig)
	if !strings.Contains(got, "rest ...int)") || !strings.Contains(got, "Rows(p, first, rest...)") {
		t.Fatalf("variadic not forwarded:\n%s", got)
	}
	f.build(t)
}

func TestGenGoFailsOnUnresolvedQualifier(t *testing.T) {
	f := newImportFixture(t)
	s, err := parseComponentSig("templ Card(p Props, a nope.N, b cfg.T)")
	if err != nil {
		t.Fatal(err)
	}
	d, err := islandConstructor(Island{Tag: "x-card", PropsType: "Props", Component: "Card"}, s)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.dir, "views", "card_tc.go")
	_, unresolved, err := writeGenGo(path, []byte("package views\n"), []string{d})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(unresolved, ",") != "cfg,nope" {
		t.Fatalf("unresolved = %v", unresolved)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "views", "cfg.go"), []byte("package views\n\nvar cfg struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = checkUnresolved([]unresolvedQuals{{rel: "views/card.templ", genPath: path, names: unresolved}})
	if err == nil || !strings.Contains(err.Error(), "views/card.templ: nope used") || strings.Contains(err.Error(), "cfg") {
		t.Fatalf("got %v", err)
	}
}
