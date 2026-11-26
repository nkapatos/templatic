package templatic

import (
	"context"
	"net/http"

	"github.com/nkapatos/templatic/internal/pagestate"
	"github.com/nkapatos/templatic/live"
)

// App wraps mux in WithAssets(src) and WithPageTokens(world), outermost
// first; a nil world or src skips its layer.
func App(world *live.World, src *ManifestSource, mux http.Handler) http.Handler {
	h := mux
	if world != nil {
		h = WithPageTokens(world)(h)
	}
	if src != nil {
		h = WithAssets(src)(h)
	}
	return h
}

// WithAssets makes src the manifest of every page rendered by the wrapped
// handler.
func WithAssets(src *ManifestSource) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			s, r := pagestate.Install(r)
			s.Manifests = src
			next.ServeHTTP(rw, r)
		})
	}
}

// WithPageTokens makes a lazily minted page token available to pages
// rendered by the wrapped handler.
func WithPageTokens(w *live.World) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			s, r := pagestate.Install(r)
			s.Tokens = pagestate.NewTokens(w, r)
			next.ServeHTTP(rw, r)
		})
	}
}

// SeedPage adds token options to the request's page token. It panics if the
// token was already minted or r is not under WithPageTokens.
func SeedPage(r *http.Request, opts ...live.TokenOption) {
	s := pagestate.From(r.Context())
	if s == nil || s.Tokens == nil {
		panic("templatic: SeedPage outside templatic.WithPageTokens")
	}
	if !s.Tokens.Seed(opts...) {
		panic("templatic: SeedPage after the page token was minted")
	}
}

// PageToken returns the page token for ctx, minting it on first call, or ""
// outside WithPageTokens.
func PageToken(ctx context.Context) string {
	if s := pagestate.From(ctx); s != nil && s.Tokens != nil {
		return s.Tokens.Mint()
	}
	return ""
}
