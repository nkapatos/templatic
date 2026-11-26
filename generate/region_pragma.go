package generate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	parser "github.com/a-h/templ/parser/v2"
)

var regionPragma = regexp.MustCompile(`(?m)^//templatic:region\b(.*)$`)

var goConstRef = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*\.)?[A-Za-z_][A-Za-z0-9_]*$`)

type regionSpec struct {
	topicExpr string
	topic     string
	component string
}

func parseRegionPragma(expression string) (spec regionSpec, line string, ok bool, err error) {
	m := regionPragma.FindStringSubmatch(expression)
	if m == nil {
		return regionSpec{}, "", false, nil
	}
	line = strings.TrimSpace(m[0])
	topics := 0
	for _, field := range strings.Fields(m[1]) {
		key, value, found := strings.Cut(field, "=")
		if !found || value == "" {
			return regionSpec{}, line, false, fmt.Errorf("%s: field %q missing =value", line, key)
		}
		switch key {
		case "topic":
			topics++
			if !validTopicName(value) {
				return regionSpec{}, line, false, fmt.Errorf("%s: topic %q must match [A-Za-z0-9_.-]+", line, value)
			}
			spec.topic, spec.topicExpr = value, strconv.Quote(value)
		case "topic-of":
			topics++
			if !goConstRef.MatchString(value) {
				return regionSpec{}, line, false, fmt.Errorf("%s: topic-of=%q must name a Go const (Name or pkg.Name)", line, value)
			}
			spec.topicExpr = value
		default:
			return regionSpec{}, line, false, fmt.Errorf("%s: unknown field %q", line, key)
		}
	}
	if topics != 1 {
		return regionSpec{}, line, false, fmt.Errorf("%s: exactly one of topic= or topic-of= is required", line)
	}
	return spec, line, true, nil
}

func scanRegions(file string, tf *parser.TemplateFile, islands map[int]Island) (map[int]regionSpec, error) {
	specs := map[int]regionSpec{}
	for i, node := range tf.Nodes {
		expr, ok := node.(*parser.TemplateFileGoExpression)
		if !ok {
			continue
		}
		spec, line, found, err := parseRegionPragma(expr.Expression.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if !found {
			continue
		}
		for j := i + 1; j < len(tf.Nodes); j++ {
			comp, ok := tf.Nodes[j].(*parser.HTMLTemplate)
			if !ok {
				continue
			}
			if _, taken := islands[j]; taken {
				return nil, fmt.Errorf("%s: %s: component already carries a pragma", file, line)
			}
			if _, taken := specs[j]; taken {
				return nil, fmt.Errorf("%s: %s: component already carries a pragma", file, line)
			}
			spec.component = componentName(comp)
			specs[j] = spec
			break
		}
	}
	return specs, nil
}

// regionDecl's renderer binding must not be a var initializer: that is an init cycle when the component renders its own region.
func regionDecl(spec regionSpec, sig goSig) (string, error) {
	name := sig.name
	var b strings.Builder
	model, renderer := "", ""
	switch len(sig.params) {
	case 0:
		model = "struct{}"
		renderer = "func(struct{}) templ.Component { return " + name + "() }"
	case 1:
		if strings.HasPrefix(sig.params[0].typ, "...") {
			return "", fmt.Errorf("%s: variadic params are not supported by //templatic:region", name)
		}
		model, renderer = sig.params[0].typ, name
	default:
		model = name + "Args"
		var fields, call []string
		for _, p := range sig.params {
			if strings.HasPrefix(p.typ, "...") {
				return "", fmt.Errorf("%s: variadic params are not supported by //templatic:region", name)
			}
			f := exportName(p.name)
			fields = append(fields, "\t"+f+" "+p.typ)
			call = append(call, "a."+f)
		}
		fmt.Fprintf(&b, "// %s are %s's params: the model of %sRegion.\n", model, name, name)
		fmt.Fprintf(&b, "type %s struct {\n%s\n}\n\n", model, strings.Join(fields, "\n"))
		renderer = fmt.Sprintf("func(a %s) templ.Component { return %s(%s) }", model, name, strings.Join(call, ", "))
	}
	fmt.Fprintf(&b, "// %sRegion is the session region rendered by %s (//templatic:region).\n", name, name)
	fmt.Fprintf(&b, "var %sRegion = live.DeclareSessionRegion[%s](%s)\n\n", name, model, spec.topicExpr)
	fmt.Fprintf(&b, "func init() { %sRegion.Renders(%s) }\n", name, renderer)
	return b.String(), nil
}

func exportName(s string) string {
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
