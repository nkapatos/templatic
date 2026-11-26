package pages

import (
	"net/http"
	"strings"
)

// Route is one page route in router-neutral form.
type Route struct {
	// Method is the HTTP method, such as "GET".
	Method string
	// Path is the route path with {name} segments; "/" matches only the root.
	Path string
	// Params names the parameter segments of Path, in order.
	Params []string
	// Handler serves the route.
	Handler http.Handler
}

// ServeMuxPattern returns the route as a net/http ServeMux pattern.
func (r Route) ServeMuxPattern() string {
	p := r.Path
	if p == "/" {
		p = "/{$}"
	}
	return r.Method + " " + p
}

// PathWith renders Path with each {name} segment replaced by param(name).
func (r Route) PathWith(param func(name string) string) string {
	segs := strings.Split(r.Path, "/")
	for i, s := range segs {
		if len(s) > 2 && s[0] == '{' && s[len(s)-1] == '}' {
			segs[i] = param(s[1 : len(s)-1])
		}
	}
	return strings.Join(segs, "/")
}
