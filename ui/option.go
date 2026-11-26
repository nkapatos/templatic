package ui

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/a-h/templ"
)

// Option configures a component's element: Swaps, Tail, Class, Attrs or
// Multipart where the component supports it.
type Option func(*elemConfig)

type elemConfig struct {
	swaps     string
	tail      bool
	multipart bool
	class     string
	attrs     templ.Attributes
	failed    *failedForm
}

func buildConfig(opts []Option) elemConfig {
	var c elemConfig
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return c
}

// Class adds CSS classes to the element; repeated calls accumulate.
func Class(classes ...string) Option {
	return func(c *elemConfig) {
		for _, cl := range classes {
			if cl = strings.TrimSpace(cl); cl != "" {
				c.class = strings.TrimSpace(c.class + " " + cl)
			}
		}
	}
}

// Attrs adds attributes to the element: a string is escaped, true renders a
// bare attribute, false omits it. It panics on data-tc-* markers and
// attributes the element owns.
func Attrs(a templ.Attributes) Option {
	for k := range a {
		l := strings.ToLower(k)
		if strings.HasPrefix(l, "data-tc") || l == "href" || l == "type" || l == "class" || l == "action" || l == "method" || l == "enctype" || !validAttrName(l) {
			panic("templatic: Attrs cannot set " + k)
		}
	}
	return func(c *elemConfig) {
		if c.attrs == nil {
			c.attrs = templ.Attributes{}
		}
		for k, v := range a {
			c.attrs[k] = v
		}
	}
}

func validAttrName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '-' || r == '_' || r == ':' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func (c elemConfig) writeExtras(w io.Writer) error {
	if c.class != "" {
		if _, err := io.WriteString(w, ` class="`+templ.EscapeString(c.class)+`"`); err != nil {
			return err
		}
	}
	if len(c.attrs) == 0 {
		return nil
	}
	for _, k := range slices.Sorted(maps.Keys(c.attrs)) {
		var err error
		switch v := c.attrs[k].(type) {
		case bool:
			if v {
				_, err = io.WriteString(w, " "+k)
			}
		case nil:
		default:
			_, err = io.WriteString(w, " "+k+`="`+templ.EscapeString(fmt.Sprint(v))+`"`)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
