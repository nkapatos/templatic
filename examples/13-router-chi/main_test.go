package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/live"

	"examples/router-chi/internal/items"
)

func newApp() http.Handler {
	w := live.NewWorld()
	return routes(w, items.Register(w))
}

// The route's {id} wins over a forged id field: the like goes to item 1.
func TestLikeFormUsesThePathID(t *testing.T) {
	h := newApp()
	r := httptest.NewRequest("POST", "/items/1/like", strings.NewReader(url.Values{"id": {"2"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/items/1" {
		t.Fatalf("got %d to %q, want 303 to /items/1", rec.Code, rec.Header().Get("Location"))
	}
}

func TestUnknownItemIs404(t *testing.T) {
	h := newApp()
	for _, r := range []*http.Request{
		httptest.NewRequest("GET", "/items/9", nil),
		httptest.NewRequest("POST", "/items/9/like", strings.NewReader("id=9")),
	} {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", r.Method, r.URL, rec.Code)
		}
	}
}
