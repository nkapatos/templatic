// Package pages is the request-time side of the experimental pages router.
//
// Experimental: this package and [github.com/nkapatos/templatic/experimental/routes]
// may change or be removed without notice. Nothing else in templatic depends
// on them.
//
// A page is a templ component that takes a [Ctx] and returns a
// [github.com/a-h/templ.Component]. [Page] turns one into a
// [net/http.Handler] that renders it, optionally inside a layout
// ([WithLayout]), and sets Cache-Control: no-cache. A [Route] describes a
// page in a router-neutral way: method, path with {name} segments, parameter
// names and handler. [Route.ServeMuxPattern] gives the net/http pattern; for
// another router, read Method, Path and Params and mount Handler yourself.
//
// [Ctx] carries the request: [Ctx.Request], [Ctx.Context], [Ctx.Path] for path
// parameters and [Ctx.Query] for query parameters. Path parameters are read
// with Request.PathValue, so routers other than net/http (and chi 5.1 or
// later) must call SetPathValue.
//
// The routes package generates the []Route for a directory of pages.
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
