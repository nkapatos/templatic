package routes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutePattern(t *testing.T) {
	cases := []struct {
		file   string
		path   string
		params []string
	}{
		{"index.templ", "/", nil},
		{"about.templ", "/about", nil},
		{"users/index.templ", "/users", nil},
		{"users/id_.templ", "/users/{id}", []string{"id"}},
		{"users/id_/posts/index.templ", "/users/{id}/posts", []string{"id"}},
		{"blog/post-id_.templ", "/blog/{post-id}", []string{"post-id"}},
		{"org/org_/repo_.templ", "/org/{org}/{repo}", []string{"org", "repo"}},
	}
	for _, c := range cases {
		method, path, params, err := routePattern(c.file)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if method != "GET" || path != c.path || strings.Join(params, ",") != strings.Join(c.params, ",") {
			t.Errorf("%s: got %s %q %v, want GET %q %v", c.file, method, path, params, c.path, c.params)
		}
	}
}

func TestRoutePatternRejectsBrackets(t *testing.T) {
	if _, _, _, err := routePattern("users/[id].templ"); err == nil {
		t.Fatal("expected an error for [id].templ")
	} else if !strings.Contains(err.Error(), "trailing underscore") {
		t.Fatalf("error should point at the convention, got: %v", err)
	}
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.27.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestScan(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"pages/index.templ": `package pages

import "github.com/nkapatos/templatic/experimental/pages"

templ Index(c pages.Ctx) {
	<h1>{ c.Query("q") }</h1>
}
`,
		"pages/users/id_.templ": `package users

import "github.com/nkapatos/templatic/experimental/pages"

templ ShowUser(c pages.Ctx) {
	<h1>{ c.Path("id") }</h1>
}
`,
	})

	tbl, err := Scan(filepath.Join(dir, "pages"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if tbl.Package != "pages" {
		t.Errorf("package = %q, want pages", tbl.Package)
	}
	if len(tbl.Imports) != 1 || tbl.Imports[0].Alias != "users" ||
		tbl.Imports[0].Path != "example.com/app/pages/users" {
		t.Errorf("imports = %+v", tbl.Imports)
	}
	if len(tbl.Pages) != 2 {
		t.Fatalf("pages = %+v, want 2", tbl.Pages)
	}
	byPattern := map[string]Page{}
	for _, p := range tbl.Pages {
		byPattern[p.Path] = p
	}
	if p := byPattern["/"]; p.Component != "Index" || p.Qualifier != "" {
		t.Errorf("root page = %+v", p)
	}
	if p := byPattern["/users/{id}"]; p.Component != "ShowUser" || p.Qualifier != "users" {
		t.Errorf("user page = %+v", p)
	}

	src := Generate(tbl)
	for _, want := range []string{
		"package pages",
		`"example.com/app/pages/users"`,
		"func Routes(opts ...pages.PageOption) []pages.Route {",
		`{Method: "GET", Path: "/", Params: nil, Handler: pages.Page(Index, opts...)}`,
		`{Method: "GET", Path: "/users/{id}", Params: []string{"id"}, Handler: pages.Page(users.ShowUser, opts...)}`,
		"func Register(mux *http.ServeMux, opts ...pages.PageOption) {",
		"mux.Handle(rt.ServeMuxPattern(), rt.Handler)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source missing %q\n%s", want, src)
		}
	}
}

func TestScanPageSignatureErrors(t *testing.T) {
	cases := map[string]string{
		"wrong parameter type": `package pages

import "github.com/nkapatos/templatic/experimental/pages"

templ Index(c string) {
	<h1>x</h1>
}
`,
		"ctx collision": `package pages

import "github.com/nkapatos/templatic/experimental/pages"

templ Index(ctx pages.Ctx) {
	<h1>x</h1>
}
`,
		"two components": `package pages

import "github.com/nkapatos/templatic/experimental/pages"

templ A(c pages.Ctx) {
	<h1>a</h1>
}

templ B(c pages.Ctx) {
	<h1>b</h1>
}
`,
		"underscore file": `package pages

templ A(c pages.Ctx) {
	<h1>a</h1>
}
`,
		"helper file": `package pages

templ card(title string) {
	<div>{ title }</div>
}
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			file := "pages/index.templ"
			switch name {
			case "underscore file":
				file = "pages/_index.templ"
			case "helper file":
				file = "pages/helpers.templ"
			}
			dir := writeModule(t, map[string]string{file: body})
			if _, err := Scan(filepath.Join(dir, "pages")); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
