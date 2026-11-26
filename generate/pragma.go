package generate

import (
	"fmt"
	"regexp"
	"strings"

	parser "github.com/a-h/templ/parser/v2"

	"github.com/nkapatos/templatic/internal/markup"
)

var islandPragma = regexp.MustCompile(`(?m)^//templatic:island\b(.*)$`)

// Bind pushes the cell Topic's value into the island's props Field.
type Bind struct {
	Field string
	Topic string
}

func parseIslandPragma(expression string) (tag, props, variant string, binds []Bind, pragmaLine string, ok bool, err error) {
	m := islandPragma.FindStringSubmatch(expression)
	if m == nil {
		return "", "", "", nil, "", false, nil
	}
	pragmaLine = strings.TrimSpace(m[0])
	for _, field := range strings.Fields(m[1]) {
		key, value, found := strings.Cut(field, "=")
		if !found {
			return "", "", "", nil, "", false, fmt.Errorf("%s: field %q missing =value", pragmaLine, key)
		}
		switch key {
		case "tag":
			tag = value
		case "props":
			props = value
		case "variant":
			variant = value
		case "bind":
			b, berr := parseBind(value)
			if berr != nil {
				return "", "", "", nil, "", false, fmt.Errorf("%s: %w", pragmaLine, berr)
			}
			binds = append(binds, b)
		default:
			return "", "", "", nil, "", false, fmt.Errorf("%s: unknown field %q", pragmaLine, key)
		}
	}
	if tag == "" || props == "" {
		return "", "", "", nil, "", false, fmt.Errorf("%s: tag= and props= are required", pragmaLine)
	}
	if !markup.ValidTag(tag) {
		return "", "", "", nil, "", false, fmt.Errorf("%s: tag must be lowercase kebab starting with a letter, with at least one dash", pragmaLine)
	}
	switch variant {
	case "", "hooks", "lit":
	default:
		return "", "", "", nil, "", false, fmt.Errorf("%s: variant must be hooks or lit", pragmaLine)
	}
	// Binds are lit-only: a hooks island keeps its SSR markup, so a reactive prop has nothing to attach to.
	if len(binds) > 0 && variant != "lit" {
		return "", "", "", nil, "", false, fmt.Errorf("%s: bind= requires variant=lit (v1: hooks islands keep server-rendered markup)", pragmaLine)
	}
	return tag, props, variant, binds, pragmaLine, true, nil
}

func parseBind(value string) (Bind, error) {
	field, topic, found := strings.Cut(value, ":")
	if !found || field == "" || topic == "" {
		return Bind{}, fmt.Errorf("bind=%q must be field:topic", value)
	}
	if !validTopicName(topic) {
		return Bind{}, fmt.Errorf("bind topic %q must match [A-Za-z0-9_.-]+", topic)
	}
	if !validJSONField(field) {
		return Bind{}, fmt.Errorf("bind field %q must be the props struct's json field name", field)
	}
	return Bind{Field: field, Topic: topic}, nil
}

// validTopicName mirrors the wire identifier grammar; it is a local copy so the pass does not import live.
func validTopicName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

func validJSONField(field string) bool {
	if field == "" {
		return false
	}
	for _, r := range field {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// scanIslands pairs each pragma with the next component, so the pragma must sit directly above it.
func scanIslands(file string, tf *parser.TemplateFile) (map[int]Island, error) {
	specs := map[int]Island{}
	for i, node := range tf.Nodes {
		expr, ok := node.(*parser.TemplateFileGoExpression)
		if !ok {
			continue
		}
		tag, props, variant, binds, line, found, err := parseIslandPragma(expr.Expression.Value)
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
			if _, taken := specs[j]; taken {
				return nil, fmt.Errorf("%s: %s: component already carries a pragma", file, line)
			}
			specs[j] = Island{
				Tag:       tag,
				PropsType: props,
				Variant:   variant,
				Binds:     binds,
				Component: componentName(comp),
				File:      file,
			}
			break
		}
	}
	return specs, nil
}

func componentName(comp *parser.HTMLTemplate) string {
	value := comp.Expression.Value
	value = strings.TrimPrefix(value, "templ")
	value = strings.TrimSpace(value)
	if idx := strings.IndexFunc(value, func(r rune) bool {
		return r == '(' || r == '[' || r == ' ' || r == '\t'
	}); idx >= 0 {
		value = value[:idx]
	}
	return value
}
