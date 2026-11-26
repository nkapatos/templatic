// Package routes maps a directory of templ pages to a Go route table. It is experimental.
//
// Experimental: this package and [github.com/nkapatos/templatic/experimental/pages]
// may change or be removed without notice. Nothing else in templatic depends
// on them.
//
// [Scan] reads the .templ files under a pages directory and returns a
// [Table], one [Page] per file. Every file is one route and must follow
// these conventions, or Scan returns an error:
//
//   - The file path is the route path: users/list.templ is /users/list, and
//     index.templ is the directory's own path.
//   - A trailing underscore marks a path parameter: users/id_.templ is
//     /users/{id}. Brackets are rejected because Go import paths cannot
//     contain them.
//   - The file defines exactly one exported component, and it takes exactly
//     one parameter, a pages.Ctx.
//   - File names do not start with "_" or ".".
//
// All routes are GET. [Generate] renders a Table as Go source with a
// Routes function returning []pages.Route and a Register function for a
// net/http ServeMux; [Write] does Scan and writes the result to
// routes_gen.go in the pages directory.
//
//	t, err := routes.Write("pages")
package routes

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/a-h/templ/parser/v2"
)

// Page is one routable page file.
type Page struct {
	// File is the .templ path relative to the pages root.
	File string
	// Component is the exported templ component that renders the page.
	Component string
	// Qualifier is the package alias used in generated code.
	Qualifier string
	// Method is the HTTP method.
	Method string
	// Path is the router-neutral path with {name} segments.
	Path string
	// Params names Path's parameter segments, in order.
	Params []string
}

// Table is the scan result for a pages directory.
type Table struct {
	// Module is the Go module path owning the pages directory.
	Module string
	// Package is the Go package name of the pages root.
	Package string
	// Imports are the subpackages the generated table must import.
	Imports []Import
	Pages   []Page
}

// Import is an emitted import of a page subpackage.
type Import struct {
	Alias string
	Path  string
}

// Scan builds the route table from the .templ files under dir, sorted by
// path, and fails if a file breaks the page conventions. It writes nothing.
func Scan(dir string) (Table, error) {
	var t Table

	module, moduleRoot, err := findModule(dir)
	if err != nil {
		return t, err
	}
	t.Module = module

	type fileEntry struct {
		abs string
		rel string
	}
	var files []fileEntry
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".templ" {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, fileEntry{abs: p, rel: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		return t, err
	}
	if len(files) == 0 {
		return t, fmt.Errorf("no .templ files under %s", dir)
	}
	slices.SortFunc(files, func(a, b fileEntry) int { return cmp.Compare(a.rel, b.rel) })

	pkgOfDir := map[string]string{}
	for _, f := range files {
		pkgDir := path.Dir(f.rel)
		if _, ok := pkgOfDir[pkgDir]; ok {
			continue
		}
		src, err := os.ReadFile(f.abs)
		if err != nil {
			return t, err
		}
		tf, err := parser.ParseString(string(src))
		if err != nil {
			return t, fmt.Errorf("%s: %w", f.rel, err)
		}
		pkg := strings.TrimSpace(tf.Package.Expression.Value)
		pkg = strings.TrimSpace(strings.TrimPrefix(pkg, "package"))
		if pkg == "" {
			return t, fmt.Errorf("%s: no package declaration", f.rel)
		}
		pkgOfDir[pkgDir] = pkg
	}

	if pkg := pkgOfDir["."]; pkg != "" {
		t.Package = pkg
	} else {
		t.Package = sanitizePackage(filepath.Base(filepath.Clean(dir)))
	}

	used := map[string]bool{t.Package: true}
	aliasOfDir := map[string]string{}
	var dirs []string
	for d := range pkgOfDir {
		if d != "." {
			dirs = append(dirs, d)
		}
	}
	slices.Sort(dirs)
	for _, d := range dirs {
		alias := uniqueAlias(pkgOfDir[d], used)
		aliasOfDir[d] = alias
		importPath := module + "/" + path.Join(relSlash(moduleRoot, dir), d)
		t.Imports = append(t.Imports, Import{Alias: alias, Path: importPath})
	}

	for _, f := range files {
		rel := f.rel
		base := path.Base(rel)
		if strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".") {
			return t, fmt.Errorf("%s: file names must not start with '_' or '.' — Go ignores generated files with those prefixes, so the page would never compile", rel)
		}

		src, err := os.ReadFile(f.abs)
		if err != nil {
			return t, err
		}
		tf, err := parser.ParseString(string(src))
		if err != nil {
			return t, fmt.Errorf("%s: %w", rel, err)
		}

		exported, err := exportedComponents(tf)
		if err != nil {
			return t, fmt.Errorf("%s: %w", rel, err)
		}
		if len(exported) != 1 {
			return t, fmt.Errorf("%s: every file under pages/ is a route, so it needs exactly one exported templ component; found %d (%s). Move layouts and helper components outside pages/",
				rel, len(exported), strings.Join(exported, ", "))
		}
		comp := exported[0]

		if err := checkPageSignature(tf, comp, rel); err != nil {
			return t, err
		}
		method, routePath, params, err := routePattern(rel)
		if err != nil {
			return t, fmt.Errorf("%s: %w", rel, err)
		}

		pkgDir := path.Dir(rel)
		t.Pages = append(t.Pages, Page{
			File:      rel,
			Component: comp,
			Qualifier: aliasOfDir[pkgDir],
			Method:    method,
			Path:      routePath,
			Params:    params,
		})
	}

	if len(t.Pages) == 0 {
		return t, fmt.Errorf("no pages found under %s", dir)
	}
	slices.SortFunc(t.Pages, func(a, b Page) int { return cmp.Compare(a.Path, b.Path) })
	return t, nil
}

func routePattern(rel string) (method, routePath string, params []string, err error) {
	p := strings.TrimSuffix(rel, ".templ")
	segments := strings.Split(p, "/")

	if last := segments[len(segments)-1]; last == "index" {
		segments = segments[:len(segments)-1]
	}
	for i, s := range segments {
		switch {
		case strings.HasPrefix(s, "[") || strings.ContainsAny(s, "[]{}"):
			return "", "", nil, fmt.Errorf("dynamic segments use a trailing underscore (e.g. users/id_.templ), not %q: "+
				"brackets break Go import paths (templ generates <name>_templ.go and the go tool rejects paths with '[')", s)
		case len(s) > 1 && strings.HasSuffix(s, "_"):
			name := strings.TrimSuffix(s, "_")
			segments[i] = "{" + name + "}"
			params = append(params, name)
		case s == "":
			return "", "", nil, fmt.Errorf("empty path segment")
		}
	}
	return "GET", "/" + strings.Join(segments, "/"), params, nil
}

func exportedComponents(tf *parser.TemplateFile) ([]string, error) {
	var out []string
	for _, node := range tf.Nodes {
		ht, ok := node.(*parser.HTMLTemplate)
		if !ok {
			continue
		}
		name := componentName(ht.Expression.Value)
		if name == "" {
			continue
		}
		if r := []rune(name)[0]; unicode.IsUpper(r) {
			out = append(out, name)
		}
	}
	return out, nil
}

func componentName(sig string) string {
	sig = strings.TrimSpace(sig)
	if i := strings.IndexByte(sig, '('); i > 0 {
		return sig[:i]
	}
	return ""
}

const (
	ctxType      = "pages.Ctx"
	componentTyp = "templ.Component"
)

type param struct {
	name string
	typ  string
}

func params(sig string) ([]param, error) {
	list, err := paramList(sig)
	if err != nil {
		return nil, err
	}
	out := make([]param, 0, len(list))
	for _, raw := range list {
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) < 2 {
			return nil, fmt.Errorf("cannot parse parameter %q", raw)
		}
		out = append(out, param{name: fields[0], typ: strings.Join(fields[1:], " ")})
	}
	return out, nil
}

func checkParamNames(ps []param, comp, file string) error {
	for _, p := range ps {
		if p.name == "ctx" {
			return fmt.Errorf("%s: %s: parameter %q collides with the context variable templ generates; name it c (or anything but ctx)", file, comp, p.name)
		}
		if strings.HasPrefix(p.name, "templ_") {
			return fmt.Errorf("%s: %s: parameter %q uses templ's reserved templ_ prefix", file, comp, p.name)
		}
	}
	return nil
}

func checkPageSignature(tf *parser.TemplateFile, comp, file string) error {
	for _, node := range tf.Nodes {
		ht, ok := node.(*parser.HTMLTemplate)
		if !ok || componentName(ht.Expression.Value) != comp {
			continue
		}
		ps, err := params(ht.Expression.Value)
		if err != nil {
			return fmt.Errorf("%s: %s: %w", file, comp, err)
		}
		if len(ps) != 1 {
			return fmt.Errorf("%s: %s: page components take exactly one parameter (%s), found %d",
				file, comp, ctxType, len(ps))
		}
		if ps[0].typ != ctxType && ps[0].typ != "Ctx" {
			return fmt.Errorf("%s: %s: page parameter type is %q, want %s", file, comp, ps[0].typ, ctxType)
		}
		return checkParamNames(ps, comp, file)
	}
	return fmt.Errorf("%s: component %s not found", file, comp)
}

func paramList(sig string) ([]string, error) {
	open := strings.IndexByte(sig, '(')
	closing := strings.LastIndexByte(sig, ')')
	if open < 0 || closing <= open {
		return nil, fmt.Errorf("cannot read parameter list from %q", sig)
	}
	var out []string
	depth, start := 0, open+1
	for i := open + 1; i < closing; i++ {
		switch sig[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, sig[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, sig[start:closing])
	if len(out) == 1 && strings.TrimSpace(out[0]) == "" {
		return nil, nil
	}
	return out, nil
}

// Generate renders the route table as Go source.
func Generate(t Table) string {
	var b strings.Builder
	b.WriteString("// Code generated by templatic. DO NOT EDIT.\n\n")
	b.WriteString("package " + t.Package + "\n\n")
	b.WriteString("import (\n\t\"net/http\"\n\n")
	b.WriteString("\t\"github.com/nkapatos/templatic/experimental/pages\"\n")
	for _, im := range t.Imports {
		b.WriteString("\n\t" + im.Alias + " " + quote(im.Path) + "\n")
	}
	b.WriteString(")\n\n")
	b.WriteString("// Routes lists every page under this directory tree in a router-neutral\n")
	b.WriteString("// form; mount them on any router (see pages.Route). Page options\n")
	b.WriteString("// (e.g. pages.WithLayout) apply to every page.\n")
	b.WriteString("func Routes(opts ...pages.PageOption) []pages.Route {\n")
	b.WriteString("\treturn []pages.Route{\n")
	for _, p := range t.Pages {
		b.WriteString("\t\t{Method: " + quote(p.Method) + ", Path: " + quote(p.Path) + ", Params: " + stringSlice(p.Params) +
			", Handler: pages.Page(" + qualified(p) + ", opts...)},\n")
	}
	b.WriteString("\t}\n}\n\n")
	b.WriteString("// Register mounts Routes on a net/http ServeMux. Other routers use\n")
	b.WriteString("// Routes directly.\n")
	b.WriteString("func Register(mux *http.ServeMux, opts ...pages.PageOption) {\n")
	b.WriteString("\tfor _, rt := range Routes(opts...) {\n")
	b.WriteString("\t\tmux.Handle(rt.ServeMuxPattern(), rt.Handler)\n")
	b.WriteString("\t}\n}\n")
	return b.String()
}

func stringSlice(ss []string) string {
	if len(ss) == 0 {
		return "nil"
	}
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = quote(s)
	}
	return "[]string{" + strings.Join(q, ", ") + "}"
}

func qualified(p Page) string {
	if p.Qualifier == "" {
		return p.Component
	}
	return p.Qualifier + "." + p.Component
}

func quote(s string) string { return `"` + s + `"` }

// Write writes Generate's output to <dir>/routes_gen.go; on a scan error nothing is
// written.
func Write(dir string) (Table, error) {
	t, err := Scan(dir)
	if err != nil {
		return t, err
	}
	out := filepath.Join(dir, "routes_gen.go")
	if err := os.WriteFile(out, []byte(Generate(t)), 0o644); err != nil {
		return t, err
	}
	return t, nil
}

func findModule(dir string) (module, root string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if b, err := os.ReadFile(filepath.Join(d, "go.mod")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if f := strings.Fields(line); len(f) >= 2 && f[0] == "module" {
					return f[1], d, nil
				}
			}
			return "", "", fmt.Errorf("%s: no module directive", filepath.Join(d, "go.mod"))
		}
		if parent := filepath.Dir(d); parent == d {
			return "", "", fmt.Errorf("no go.mod above %s", dir)
		}
	}
}

func relSlash(moduleRoot, dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "."
	}
	rel, err := filepath.Rel(moduleRoot, abs)
	if err != nil || rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

func uniqueAlias(pkg string, used map[string]bool) string {
	base := sanitizePackage(pkg)
	alias := base
	for i := 2; used[alias] || goKeywords[alias]; i++ {
		alias = fmt.Sprintf("%s%d", base, i)
	}
	used[alias] = true
	return alias
}

func sanitizePackage(name string) string {
	var b strings.Builder
	for i, r := range name {
		switch {
		case r == '-' || r == '.' || r == '[' || r == ']' || r == '{' || r == '}':
			b.WriteRune('_')
		case unicode.IsLetter(r) || r == '_':
			b.WriteRune(r)
		case unicode.IsDigit(r):
			if i == 0 {
				b.WriteRune('p')
			}
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		s = "pages"
	}
	return s
}
