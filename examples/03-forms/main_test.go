package main

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"
)

// app wires a world and returns it with Save's handler, mounted as main mounts it.
func app(t *testing.T) (*live.World, http.Handler, *store) {
	t.Helper()
	w := live.NewWorld()
	books := newStore()
	_, save := wire(w, books)
	h := saveHandler(w, save, nil)
	mux := http.NewServeMux()
	mux.Handle("POST /books", h)
	mux.Handle("POST /books/{id}", h)
	return w, mux, books
}

func TestSaveRefusesEveryBadField(t *testing.T) {
	w, h, _ := app(t)
	s := livetest.New(t, w)

	rec := s.PostForm(h, "/books", url.Values{"title": {""}, "pages": {"0"}, "shelf": {"attic"}, "favorite": {"true"}})
	var env struct {
		Error struct {
			Message string            `json:"message"`
			Fields  map[string]string `json:"fields"`
		} `json:"error"`
	}
	if rec.Code != http.StatusBadRequest || json.Unmarshal(rec.Body.Bytes(), &env) != nil {
		t.Fatalf("status %d %q", rec.Code, rec.Body.String())
	}
	for _, field := range []string{"title", "pages", "shelf"} {
		if env.Error.Fields[field] == "" {
			t.Errorf("no error for field %q; got %v", field, env.Error.Fields)
		}
	}
	if env.Error.Message == "" {
		t.Error("no error for the whole form")
	}
}

func TestSaveUnknownBookIs404(t *testing.T) {
	w, h, _ := app(t)
	s := livetest.New(t, w)

	rec := s.PostForm(h, "/books/9", url.Values{"title": {"Dune"}, "pages": {"412"}, "shelf": {"done"}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}

func TestSaveRedrawsTheList(t *testing.T) {
	w, h, _ := app(t)
	s := livetest.New(t, w)

	if rec := s.PostForm(h, "/books", url.Values{"title": {"Dune"}, "pages": {"412"}, "shelf": {"to-read"}, "tags": {"fiction"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	s.WaitSwap("books", "Dune", time.Second)
}

func TestFormRunsOncePerSubmission(t *testing.T) {
	w, h, books := app(t)

	id := w.SubmitID(w.PageToken(httptest.NewRequest("GET", "/", nil)))
	form := url.Values{"title": {"Dune"}, "pages": {"412"}, "shelf": {"done"}, "_tc_submit": {id}}
	for range 2 {
		r := httptest.NewRequest("POST", "/books", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", rec.Code)
		}
	}
	if n := len(books.all()); n != 1 {
		t.Fatalf("%d books after a resubmission, want 1", n)
	}
}
