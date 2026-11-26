package pages_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/experimental/pages"
)

func TestRouteRendersForEveryRouterSyntax(t *testing.T) {
	cases := []struct {
		rt     pages.Route
		mux    string
		colons string
	}{
		{pages.Route{Method: "GET", Path: "/"}, "GET /{$}", "/"},
		{pages.Route{Method: "GET", Path: "/about"}, "GET /about", "/about"},
		{pages.Route{Method: "GET", Path: "/users/{id}", Params: []string{"id"}}, "GET /users/{id}", "/users/:id"},
		{pages.Route{Method: "GET", Path: "/o/{org}/{repo}/x", Params: []string{"org", "repo"}}, "GET /o/{org}/{repo}/x", "/o/:org/:repo/x"},
	}
	colon := func(n string) string { return ":" + n }
	for _, c := range cases {
		if got := c.rt.ServeMuxPattern(); got != c.mux {
			t.Errorf("%s: ServeMuxPattern = %q, want %q", c.rt.Path, got, c.mux)
		}
		if got := c.rt.PathWith(colon); got != c.colons {
			t.Errorf("%s: PathWith(:) = %q, want %q", c.rt.Path, got, c.colons)
		}
	}
}

func TestPagePathParamsAreRouterNeutral(t *testing.T) {
	page := pages.Page(func(c pages.Ctx) templ.Component {
		return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
			_, err := io.WriteString(w, "user="+c.Path("id"))
			return err
		})
	})
	rt := pages.Route{Method: "GET", Path: "/users/{id}", Params: []string{"id"}, Handler: page}

	foreign := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.URL.Path, "/users/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		r.SetPathValue(rt.Params[0], id)
		rt.Handler.ServeHTTP(w, r)
	})
	rec := httptest.NewRecorder()
	foreign.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42", nil))
	if got := rec.Body.String(); got != "user=42" {
		t.Fatalf("foreign router: body = %q, want user=42", got)
	}

	mux := http.NewServeMux()
	mux.Handle(rt.ServeMuxPattern(), rt.Handler)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/7", nil))
	if got := rec.Body.String(); got != "user=7" {
		t.Fatalf("ServeMux: body = %q, want user=7", got)
	}
}
