// Package pagestate is the per-request page state shared by middleware, Boot, Head
// and ui.Island. The root templatic package and ui use it.
package pagestate

import (
	"context"
	"net/http"
	"sync"

	"github.com/nkapatos/templatic/internal/manifest"
	"github.com/nkapatos/templatic/live"
)

type key struct{}

// State is one request's page state: the page token, the manifest source and
// the islands the page rendered.
type State struct {
	Tokens    *Tokens
	Manifests *manifest.Source

	mu      sync.Mutex
	islands []string
	links   map[string]bool
}

// Install returns r's state, adding one to r's context if absent.
func Install(r *http.Request) (*State, *http.Request) {
	if s, ok := r.Context().Value(key{}).(*State); ok {
		return s, r
	}
	s := &State{}
	return s, r.WithContext(context.WithValue(r.Context(), key{}, s))
}

// From returns ctx's state, or nil.
func From(ctx context.Context) *State {
	s, _ := ctx.Value(key{}).(*State)
	return s
}

// WithState returns ctx with a state installed if it has none.
func WithState(ctx context.Context) context.Context {
	if From(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, &State{})
}

// NoteLink records a URL written as a link; it reports false if the URL was
// already written for this request (or ctx has no state).
func NoteLink(ctx context.Context, url string) bool {
	s := From(ctx)
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.links[url] {
		return false
	}
	if s.links == nil {
		s.links = map[string]bool{}
	}
	s.links[url] = true
	return true
}

// NoteIsland records an island tag the page rendered (deduplicated).
func NoteIsland(ctx context.Context, tag string) {
	s := From(ctx)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.islands {
		if t == tag {
			return
		}
	}
	s.islands = append(s.islands, tag)
}

// RenderedIslands returns the island tags recorded so far.
func RenderedIslands(ctx context.Context) []string {
	s := From(ctx)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.islands...)
}

// Manifest returns the request's current manifest, or nil.
func Manifest(ctx context.Context) *manifest.Manifest {
	if s := From(ctx); s != nil && s.Manifests != nil {
		return s.Manifests.Current()
	}
	return nil
}

// Tokens mints a request's page token lazily, at most once.
type Tokens struct {
	world *live.World
	r     *http.Request

	mu    sync.Mutex
	seeds []live.TokenOption
	tok   string
	done  bool
}

// NewTokens returns the token minter for r.
func NewTokens(w *live.World, r *http.Request) *Tokens {
	return &Tokens{world: w, r: r}
}

// Mint returns the page token, minting it on first call.
func (t *Tokens) Mint() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.done {
		t.tok = t.world.PageToken(t.r, t.seeds...)
		t.done = true
	}
	return t.tok
}

// Seed adds token options. It reports false if the token was already minted.
func (t *Tokens) Seed(opts ...live.TokenOption) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return false
	}
	t.seeds = append(t.seeds, opts...)
	return true
}

// SubmitID mints a submission id for the page's session, minting the page
// token if needed.
func (t *Tokens) SubmitID() string { return t.world.SubmitID(t.Mint()) }
