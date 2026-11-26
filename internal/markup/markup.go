// Package markup holds attribute escaping, name grammar and cell value rendering.
package markup

import (
	"encoding/json/v2"
	"fmt"
	"strings"
)

// EscapeAttr escapes a value for a double-quoted HTML attribute context.
func EscapeAttr(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// ValidName reports whether name matches the wire identifier grammar; keep it in
// sync with live's checkTopic.
func ValidName(name string) bool {
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

// ValidTag reports whether tag is a lowercase-kebab custom element name starting with a-z.
func ValidTag(tag string) bool {
	if tag == "" || tag[0] < 'a' || tag[0] > 'z' || strings.Contains(tag, "--") {
		return false
	}
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return !strings.HasPrefix(tag, "-") && strings.Contains(tag, "-")
}

// RenderCellValue renders v as the client runtime renders live updates: strings
// raw, nil pointers and interfaces empty, other values as JSON text (nil slices
// and maps as [] and {}), unencodable values as %v.
func RenderCellValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	if s == "null" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(b, &str); err == nil {
			return str
		}
	}
	return s
}
