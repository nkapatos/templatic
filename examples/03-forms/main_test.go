package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/forms/components"
)

func TestSaveRefusesEveryBadField(t *testing.T) {
	w := live.NewWorld()
	wire(w, newStore())
	s := livetest.New(t, w)

	_, err := livetest.Submit(s, components.Save, url.Values{
		"title": {""}, "pages": {"0"}, "shelf": {"attic"}, "favorite": {"true"},
	})
	got := map[string]string{}
	for _, ie := range live.InputErrors(err) {
		got[ie.Field] = ie.Message
	}
	for _, field := range []string{"title", "pages", "shelf", ""} {
		if got[field] == "" {
			t.Errorf("no error for field %q; got %v", field, got)
		}
	}
}

func TestSaveUnknownBookIs404(t *testing.T) {
	w := live.NewWorld()
	wire(w, newStore())
	s := livetest.New(t, w)

	_, err := livetest.Call(s, components.Save, components.BookIn{ID: "9", Title: "Dune", Pages: 412, Shelf: "done"})
	var se *live.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Fatalf("err = %v, want a 404 StatusError", err)
	}
}

func TestSaveRedrawsTheList(t *testing.T) {
	w := live.NewWorld()
	wire(w, newStore())
	s := livetest.New(t, w)

	if _, err := livetest.Submit(s, components.Save, url.Values{
		"title": {"Dune"}, "pages": {"412"}, "shelf": {"to-read"}, "tags": {"fiction"},
	}); err != nil {
		t.Fatal(err)
	}
	s.WaitSwap("books", "Dune", time.Second)
}

func TestFormPOSTRunsOncePerSubmission(t *testing.T) {
	w := live.NewWorld()
	books := newStore()
	_, save := wire(w, books)
	h := components.Save.FormPOST(w, func(_ *http.Request, in components.BookIn) (string, error) {
		_, err := save(in)
		return "/", err
	}, nil)

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
