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
