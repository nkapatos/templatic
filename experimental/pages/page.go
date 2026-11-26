// Package pages is the request-time side of the experimental pages router.
package pages

import (
	"net/http"

	"github.com/a-h/templ"
)

// PageFunc is the signature templ generates for a page component.
type PageFunc func(Ctx) templ.Component

// LayoutFunc wraps a page component.
type LayoutFunc func(Ctx, templ.Component) templ.Component

type pageOptions struct {
	layout LayoutFunc
}

// PageOption customizes how a page is served.
type PageOption func(*pageOptions)

// WithLayout wraps every render of the page in layout.
func WithLayout(layout LayoutFunc) PageOption {
	return func(o *pageOptions) { o.layout = layout }
}

// Page adapts a page component to an http.Handler that serves documents
// with Cache-Control: no-cache.
func Page(page PageFunc, opts ...PageOption) http.Handler {
	var o pageOptions
	for _, opt := range opts {
		opt(&o)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		ctx := NewCtx(r)
		component := page(ctx)
		if o.layout != nil {
			component = o.layout(ctx, component)
		}
		templ.Handler(component).ServeHTTP(w, r)
	})
}
