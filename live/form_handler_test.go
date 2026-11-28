package live

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nkapatos/templatic/internal/form"
)

// jsFormReq is the runtime's POST of a form to its action URL.
func jsFormReq(sid string, vals url.Values) *http.Request {
	r := formReq(vals)
	r.Header.Set(FormHeader, "1")
	r.Header.Set(SidHeader, sid)
	return r
}

func TestFormServesTheRuntimeWithTheSession(t *testing.T) {
	var seen *Session
	w, _, cmd := liveWorld(t, "s1", nil)
	h := cmd.Form(w, func(_ *http.Request, s *Session, in noteIn) (string, error) {
		seen = s
		switch in.Title {
		case "":
			return "", Invalid("title", "required")
		case "gone":
			return "", NotFound("no such note")
		}
		return "/notes", nil
	}, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsFormReq("s1", url.Values{"title": {"a"}}))
	if rec.Code != http.StatusNoContent || seen == nil || seen.SID() != "s1" {
		t.Fatalf("JS submit: %d, session %v", rec.Code, seen)
	}
	if c, ok := w.verifySubmit(rec.Header().Get(SubmitHeader)); !ok || c.SID != "s1" {
		t.Fatal("a JS answer carries a fresh submission id")
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsFormReq("s1", url.Values{}))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"title":"required"`) {
		t.Fatalf("JS input error: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsFormReq("s1", url.Values{"title": {"gone"}}))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no such note") {
		t.Fatalf("JS status error: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsFormReq("nobody", url.Values{"title": {"a"}}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("unknown session: %d, want 409 so the runtime posts natively", rec.Code)
	}
}

func TestFormRunsOnceAcrossTransports(t *testing.T) {
	var runs atomic.Int32
	w, _, cmd := liveWorld(t, "s1", nil)
	h := cmd.Form(w, func(_ *http.Request, s *Session, _ noteIn) (string, error) {
		runs.Add(1)
		return "/notes", nil
	}, nil)
	id := w.mintSubmit("s1")
	vals := url.Values{"title": {"a"}, form.SubmitField: {id}}

	first := httptest.NewRecorder()
	h.ServeHTTP(first, jsFormReq("s1", vals))
	again := httptest.NewRecorder()
	h.ServeHTTP(again, jsFormReq("s1", vals))
	native := httptest.NewRecorder()
	h.ServeHTTP(native, formPOSTReq(id, "http://example.com/notes/1"))
	if runs.Load() != 1 || first.Code != 204 || again.Code != 204 || native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/notes/1" {
		t.Fatalf("runs %d: %d %d %d %q", runs.Load(), first.Code, again.Code, native.Code, native.Header().Get("Location"))
	}
}

func TestFormWithoutJSFindsTheLiveSessionBySubmissionID(t *testing.T) {
	var seen *Session
	w, _, cmd := liveWorld(t, "s1", nil)
	h := cmd.Form(w, func(_ *http.Request, s *Session, _ noteIn) (string, error) { seen = s; return "/", nil }, nil)
	h.ServeHTTP(httptest.NewRecorder(), formPOSTReq(w.mintSubmit("s1"), ""))
	if seen == nil || seen.SID() != "s1" {
		t.Fatalf("session %v", seen)
	}
	seen = nil
	h.ServeHTTP(httptest.NewRecorder(), formReq(url.Values{"title": {"a"}}))
	if seen != nil {
		t.Fatal("a form without a submission id has no session")
	}
}

func TestARuntimeKeyMakesASwappedFormRunOnce(t *testing.T) {
	var runs atomic.Int32
	w, ts, cmd := liveWorld(t, "s1", func(*Session, noteIn) (any, error) { runs.Add(1); return nil, nil })
	h := cmd.Form(w, func(*http.Request, *Session, noteIn) (string, error) { runs.Add(1); return "/", nil }, nil)
	key := "c.0f8e0c5e-8f0a-4c6b-9a57-2d3a4a1c9b11"
	vals := url.Values{"title": {"a"}, SubmitField: {key}}

	for range 2 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, jsFormReq("s1", vals))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("JS submit: %d", rec.Code)
		}
	}
	if runs.Load() != 1 {
		t.Fatalf("a repeated runtime key ran %d times, want 1", runs.Load())
	}
	// the same key on the command bus, as an island's send({once}) sends it
	for range 2 {
		postForm(t, ts, "s1", "note.add", "c.island-signup-0001", url.Values{"title": {"a"}})
	}
	if runs.Load() != 2 {
		t.Fatalf("a repeated send key ran %d times in all, want 2", runs.Load())
	}
	// without JavaScript there is no session to scope a runtime key to: it guards nothing
	h.ServeHTTP(httptest.NewRecorder(), formReq(vals))
	h.ServeHTTP(httptest.NewRecorder(), formReq(vals))
	if runs.Load() != 4 {
		t.Fatalf("runs %d, want 4", runs.Load())
	}
}
