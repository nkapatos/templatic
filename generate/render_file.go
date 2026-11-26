package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"strings"

	"github.com/a-h/templ/generator"
	parser "github.com/a-h/templ/parser/v2"
)

func renderFile(opts Options, rel string, s *Summary, pending *[]unresolvedQuals) ([]Island, error) {
	abs := filepath.Join(opts.Root, filepath.FromSlash(rel))
	tf, err := parser.Parse(abs)
	if err != nil {
		return nil, err
	}

	specs, err := scanIslands(rel, tf)
	if err != nil {
		return nil, err
	}
	regions, err := scanRegions(rel, tf, specs)
	if err != nil {
		return nil, err
	}

	var islands []Island
	for i, node := range tf.Nodes {
		if _, ok := node.(*parser.HTMLTemplate); !ok {
			continue
		}
		spec, isIsland := specs[i]
		if !isIsland {
			continue
		}
		extracted, err := takeover(node.(*parser.HTMLTemplate))
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", rel, spec.Component, err)
		}
		spec.Script = extracted.script
		spec.CSS = extracted.css
		spec.FileDir = filepath.ToSlash(filepath.Dir(rel))
		islands = append(islands, spec)
	}

	formatted, err := emitGo(tf, filepath.ToSlash(rel))
	if err != nil {
		return nil, err
	}

	// x.templ becomes x_templ.go as in the templ CLI; the output is formatted because templatic replaces its post-formatter.
	goPath := strings.TrimSuffix(abs, ".templ") + "_templ.go"
	if err := writeAtomic(goPath, formatted); err != nil {
		return nil, err
	}
	s.Emitted = append(s.Emitted, strings.TrimSuffix(rel, ".templ")+"_templ.go")

	var decls []string
	for i, node := range tf.Nodes {
		comp, ok := node.(*parser.HTMLTemplate)
		if !ok {
			continue
		}
		spec, isIsland := specs[i]
		region, isRegion := regions[i]
		if !isIsland && !isRegion {
			continue
		}
		sig, err := parseComponentSig(comp.Expression.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", rel, componentName(comp), err)
		}
		var d string
		if isRegion {
			d, err = regionDecl(region, sig)
		} else {
			d, err = islandConstructor(spec, sig)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		decls = append(decls, d)
	}
	genPath := strings.TrimSuffix(abs, ".templ") + "_tc.go"
	wrote, unresolved, err := writeGenGo(genPath, formatted, decls)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	if len(unresolved) > 0 {
		*pending = append(*pending, unresolvedQuals{rel: rel, genPath: genPath, names: unresolved})
	}
	if wrote {
		s.Emitted = append(s.Emitted, strings.TrimSuffix(rel, ".templ")+"_tc.go")
	}
	return islands, nil
}

func emitGo(tf *parser.TemplateFile, rel string) ([]byte, error) {
	var out bytes.Buffer
	if _, err := generator.Generate(tf, &out,
		generator.WithVersion(templVersion),
		generator.WithFileName(rel),
	); err != nil {
		return nil, err
	}
	return format.Source(out.Bytes())
}

type sfcTakeover struct {
	script string
	css    string
}

func takeover(comp *parser.HTMLTemplate) (sfcTakeover, error) {
	var t sfcTakeover
	var scripts, styles []string

	stripped := make([]parser.Node, 0, len(comp.Children))
	for _, n := range comp.Children {
		keep, script, css, err := takeNode(n)
		if err != nil {
			return t, err
		}
		if script != "" {
			scripts = append(scripts, script)
		}
		if css != "" {
			styles = append(styles, css)
		}
		if keep {
			stripped = append(stripped, n)
		}
	}
	comp.Children = stripped

	t.script = strings.Join(scripts, "\n\n")
	t.css = strings.Join(styles, "\n")
	return t, nil
}

func isCodeScript(el *parser.ScriptElement) bool {
	for _, a := range el.Attributes {
		switch a := a.(type) {
		case *parser.ConstantAttribute:
			if a.Key.String() != "type" {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(a.Value)) {
			case "", "module", "text/javascript", "application/javascript":
				return true
			}
			return false
		case *parser.ExpressionAttribute:
			if a.Key.String() == "type" {
				return false
			}
		}
	}
	return true
}

func takeNode(n parser.Node) (keep bool, script string, css string, err error) {
	switch el := n.(type) {
	case *parser.ScriptElement:
		if !isCodeScript(el) {
			return true, "", "", nil
		}
		var sb strings.Builder
		for _, part := range el.Contents {
			if part.Value == nil {
				return false, "", "", fmt.Errorf("script block uses Go interpolation; islands take raw TS only")
			}
			sb.WriteString(*part.Value)
		}
		return false, sb.String(), "", nil
	case *parser.Element:
		if el.Name == "style" {
			css, err := elementText(el)
			return false, "", css, err
		}
		var (
			scripts, styles []string
			children        = el.Children[:0]
		)
		for _, c := range el.Children {
			keep, script, css, err := takeNode(c)
			if err != nil {
				return true, "", "", err
			}
			if script != "" {
				scripts = append(scripts, script)
			}
			if css != "" {
				styles = append(styles, css)
			}
			if keep {
				children = append(children, c)
			}
		}
		el.Children = children
		return true, strings.Join(scripts, "\n\n"), strings.Join(styles, "\n"), nil
	case *parser.RawElement:
		if el.Name == "style" {
			return false, "", el.Contents, nil
		}
		return true, "", "", nil
	default:
		return true, "", "", nil
	}
}

func elementText(el *parser.Element) (string, error) {
	var sb strings.Builder
	for _, c := range el.Children {
		if t, ok := c.(*parser.Text); ok {
			sb.WriteString(t.Value)
		}
	}
	return sb.String(), nil
}
