package generate

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const liveImport = templaticImport + "/live"

// program is the app module type-checked once per run, after templ generation.
type program struct {
	root  string
	fset  *token.FileSet
	pkgs  []*packages.Package
	calls []liveCall
}

// liveCall is a call of a function declared in the live package.
type liveCall struct {
	pkg   *packages.Package
	fn    string
	call  *ast.CallExpr
	targs *types.TypeList
	where string
}

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedImports |
	packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo

// loadProgram type-checks every package of the module at root. Errors in a package that imports live fail the load; other packages' errors are the compiler's to report.
func loadProgram(root string) (*program, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	fset := token.NewFileSet()
	pkgs, err := packages.Load(&packages.Config{Mode: loadMode, Dir: abs, Fset: fset}, "./...")
	if err != nil {
		return nil, fmt.Errorf("generate: load packages: %w", err)
	}
	slices.SortFunc(pkgs, func(a, b *packages.Package) int { return strings.Compare(a.PkgPath, b.PkgPath) })
	p := &program{root: abs, fset: fset, pkgs: pkgs}
	for _, pkg := range pkgs {
		if pkg.Imports[liveImport] == nil {
			continue
		}
		if err := pkgErr(pkg); err != nil {
			return nil, err
		}
		p.scanLiveCalls(pkg)
	}
	return p, nil
}

func pkgErr(pkg *packages.Package) error {
	if len(pkg.Errors) == 0 {
		return nil
	}
	msgs := make([]string, len(pkg.Errors))
	for i, e := range pkg.Errors {
		msgs[i] = e.Error()
	}
	return fmt.Errorf("generate: package %s: %s", pkg.PkgPath, strings.Join(msgs, "\n"))
}

func (p *program) scanLiveCalls(pkg *packages.Package) {
	for _, f := range pkg.Syntax {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn := typeutil.StaticCallee(pkg.TypesInfo, call)
			if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != liveImport || fn.Signature().Recv() != nil {
				return true
			}
			lc := liveCall{pkg: pkg, fn: fn.Name(), call: call, where: p.where(call)}
			if id := calleeIdent(call.Fun); id != nil {
				if inst, ok := pkg.TypesInfo.Instances[id]; ok {
					lc.targs = inst.TypeArgs
				}
			}
			p.calls = append(p.calls, lc)
			return true
		})
	}
}

func calleeIdent(e ast.Expr) *ast.Ident {
	switch v := ast.Unparen(e).(type) {
	case *ast.Ident:
		return v
	case *ast.SelectorExpr:
		return v.Sel
	case *ast.IndexExpr:
		return calleeIdent(v.X)
	case *ast.IndexListExpr:
		return calleeIdent(v.X)
	}
	return nil
}

func (p *program) where(n ast.Node) string {
	pos := p.fset.Position(n.Pos())
	rel, err := filepath.Rel(p.root, pos.Filename)
	if err != nil {
		rel = pos.Filename
	}
	return filepath.ToSlash(rel) + ":" + fmt.Sprint(pos.Line)
}

// constString is argument i of the call when the type checker evaluated it to a string constant.
func (c liveCall) constString(i int) (string, bool) {
	if i >= len(c.call.Args) {
		return "", false
	}
	tv, ok := c.pkg.TypesInfo.Types[c.call.Args[i]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

func (p *program) pkgInDir(dir string) *packages.Package {
	for _, pkg := range p.pkgs {
		if len(pkg.GoFiles) > 0 && filepath.Dir(pkg.GoFiles[0]) == dir {
			return pkg
		}
	}
	return nil
}

// pkgErr is the load errors of the module package with the given path, nil for packages outside the module.
func (p *program) pkgErr(path string) error {
	for _, pkg := range p.pkgs {
		if pkg.PkgPath == path {
			return pkgErr(pkg)
		}
	}
	return nil
}
