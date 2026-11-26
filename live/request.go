package live

import (
	"maps"
	"net/http"
	"slices"
)

// RequestInfo holds the allowlisted facts of the HTTP request behind a session or
// command.
type RequestInfo struct {
	// Header holds only allowlisted headers.
	Header http.Header
	// Cookies holds only allowlisted cookies.
	Cookies []*http.Cookie
	// RemoteAddr is the request's peer address (a proxy's, behind one).
	RemoteAddr string
	// Params are the server-attested page-token values.
	Params map[string]string
}

// Cookie returns the named allowlisted cookie.
func (ri RequestInfo) Cookie(name string) (*http.Cookie, bool) {
	for _, c := range ri.Cookies {
		if c.Name == name {
			return c, true
		}
	}
	return nil, false
}

func (ri RequestInfo) clone() RequestInfo {
	out := RequestInfo{
		Header:     ri.Header.Clone(),
		RemoteAddr: ri.RemoteAddr,
		Params:     maps.Clone(ri.Params),
	}
	for _, c := range ri.Cookies {
		cc := *c
		out.Cookies = append(out.Cookies, &cc)
	}
	return out
}

// Credential headers (Authorization, Cookie) are never included.
var defaultRequestHeaders = []string{
	"User-Agent", "Accept-Language", "Referer",
	"X-Forwarded-For", "X-Forwarded-Proto", "X-Real-Ip",
}

// WithRequestHeaders adds header names to the allowlist captured into RequestInfo.
func WithRequestHeaders(names ...string) Option {
	return func(w *World) {
		for _, n := range names {
			w.reqHeaders = append(w.reqHeaders, http.CanonicalHeaderKey(n))
		}
	}
}

// WithRequestCookies allowlists cookie names captured into RequestInfo.
func WithRequestCookies(names ...string) Option {
	return func(w *World) { w.reqCookies = append(w.reqCookies, names...) }
}

func (w *World) captureRequest(r *http.Request, params map[string]string) RequestInfo {
	ri := RequestInfo{Header: http.Header{}, RemoteAddr: r.RemoteAddr, Params: maps.Clone(params)}
	for _, name := range defaultRequestHeaders {
		if v := r.Header.Values(name); len(v) > 0 {
			ri.Header[name] = slices.Clone(v)
		}
	}
	for _, name := range w.reqHeaders {
		if v := r.Header.Values(name); len(v) > 0 {
			ri.Header[name] = slices.Clone(v)
		}
	}
	for _, name := range w.reqCookies {
		if c, err := r.Cookie(name); err == nil {
			cc := *c
			ri.Cookies = append(ri.Cookies, &cc)
		}
	}
	return ri
}

// Request returns the facts of the request that opened or last reattached the
// session's stream.
func (s *Session) Request() RequestInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.req.clone()
}

// CommandRequest returns the facts of the command POST being dispatched.
func (s *Session) CommandRequest() RequestInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmdReq.clone()
}
