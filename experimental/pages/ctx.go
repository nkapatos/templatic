package pages

import (
	"context"
	"net/http"
)

// Ctx is the argument page components receive; the zero Ctx is usable and
// returns nil, background and "" from its accessors. Experimental.
type Ctx struct {
	Req *http.Request
}

// NewCtx wraps r.
func NewCtx(r *http.Request) Ctx {
	return Ctx{Req: r}
}

// Request returns the underlying request.
func (c Ctx) Request() *http.Request {
	return c.Req
}

// Context returns the request context.
func (c Ctx) Context() context.Context {
	if c.Req == nil {
		return context.Background()
	}
	return c.Req.Context()
}

// Path returns a route path parameter; routers other than net/http and chi v5.1+
// must call r.SetPathValue.
func (c Ctx) Path(name string) string {
	if c.Req == nil {
		return ""
	}
	return c.Req.PathValue(name)
}

// Query returns the first value of a query parameter, or "".
func (c Ctx) Query(name string) string {
	if c.Req == nil {
		return ""
	}
	return c.Req.URL.Query().Get(name)
}
