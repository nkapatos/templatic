package live

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nkapatos/templatic/internal/form"
)

type noteIn struct {
	Title string `json:"title"`
}

// postSubmit raises action with title as its JSON input and submit as its run-once key.
func postSubmit(t *testing.T, ts *httptest.Server, sid, action, submit, title string) (*http.Response, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"sid": sid, "action": action, "args": map[string]string{"title": title}, "submit": submit})
	resp, err := http.Post(ts.URL+"/live/commands", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func liveWorld(t *testing.T, sid string, fn func(*Session, noteIn) (any, error)) (*World, *httptest.Server, *Command[noteIn]) {
	t.Helper()
	w := NewWorld()
	cmd := NewCommand[noteIn]("note.add")
	cmd.Handle(w, fn)
	ts := serveWorld(t, w)
	st := openStream(t, ts, sid)
	t.Cleanup(func() { st.Close() })
	st.discardUntilReady(t)
	return w, ts, cmd
}

func TestSubmitRepeatAnswersFirstOutcome(t *testing.T) {
	var runs atomic.Int32
	w, ts, _ := liveWorld(t, "s1", func(_ *Session, in noteIn) (any, error) {
		return map[string]any{"n": runs.Add(1)}, nil
	})
	id := w.mintSubmit("s1")
	r1, b1 := postSubmit(t, ts, "s1", "note.add", id, "a")
	r2, b2 := postSubmit(t, ts, "s1", "note.add", id, "a")
	if runs.Load() != 1 || r1.StatusCode != 200 || r2.StatusCode != 200 || b1 != b2 {
		t.Fatalf("runs %d: %d %q / %d %q", runs.Load(), r1.StatusCode, b1, r2.StatusCode, b2)
	}
	next := r1.Header.Get(SubmitHeader)
	if c, ok := w.verifySubmit(next); !ok || c.SID != "s1" || next == id {
		t.Fatalf("fresh id %q", next)
	}
	postSubmit(t, ts, "s1", "note.add", next, "a")
	if runs.Load() != 2 {
		t.Fatalf("fresh id must run again: %d", runs.Load())
	}
}

func TestSubmitFailureReleasesID(t *testing.T) {
	var runs atomic.Int32
	w, ts, _ := liveWorld(t, "s1", func(*Session, noteIn) (any, error) {
		if runs.Add(1) == 1 {
			return nil, errors.New("store down")
		}
		return nil, nil
	})
	id := w.mintSubmit("s1")
	if r, _ := postSubmit(t, ts, "s1", "note.add", id, ""); r.StatusCode != 500 {
		t.Fatalf("first: %d", r.StatusCode)
	}
	if r, _ := postSubmit(t, ts, "s1", "note.add", id, ""); r.StatusCode != 204 || runs.Load() != 2 {
		t.Fatalf("retry: %d runs %d", r.StatusCode, runs.Load())
	}
}

func TestSubmitIDIsBoundToItsSession(t *testing.T) {
	var runs atomic.Int32
	w, ts, _ := liveWorld(t, "s1", func(*Session, noteIn) (any, error) { runs.Add(1); return nil, nil })
	other := w.mintSubmit("s2")
	postSubmit(t, ts, "s1", "note.add", other, "")
	postSubmit(t, ts, "s1", "note.add", other, "")
	postSubmit(t, ts, "s1", "note.add", "forged.id", "")
	if runs.Load() != 3 {
		t.Fatalf("runs %d", runs.Load())
	}
}

func TestSubmitIDIsNotAPageToken(t *testing.T) {
	w := NewWorld()
	if _, err := w.verifyToken(w.mintSubmit("s1")); err == nil {
		t.Fatal("submission id verified as a page token")
	}
	if _, ok := w.verifySubmit(tokenFor(w, "s1")); ok {
		t.Fatal("page token verified as a submission id")
	}
}

func TestSubmitFieldIsNotInput(t *testing.T) {
	type in struct {
		Title  string `json:"title"`
		Submit string `json:"_tc_submit"`
	}
	got, err := DecodeForm[in](formReq(url.Values{"title": {"a"}, form.SubmitField: {"x"}}))
	if err != nil || got.Submit != "" || got.Title != "a" {
		t.Fatalf("%+v %v", got, err)
	}
}

func formPOSTReq(id, referer string) *http.Request {
	r := formReq(url.Values{"title": {"a"}, form.SubmitField: {id}})
	if referer != "" {
		r.Header.Set("Referer", referer)
	}
	return r
}

func TestFormDoubleSubmitRunsOnce(t *testing.T) {
	w := NewWorld()
	var runs atomic.Int32
	h := NewCommand[noteIn]("note.add").Form(w, func(*http.Request, *Session, noteIn) (string, error) {
		runs.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "/notes/1", nil
	}, nil)
	id := w.mintSubmit("s1")
	var wg sync.WaitGroup
	recs := []*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
	for _, rec := range recs {
		wg.Add(1)
		go func() { defer wg.Done(); h.ServeHTTP(rec, formPOSTReq(id, "")) }()
	}
	wg.Wait()
	for _, rec := range recs {
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/notes/1" {
			t.Fatalf("%d %q", rec.Code, rec.Header().Get("Location"))
		}
	}
	if runs.Load() != 1 {
		t.Fatalf("runs %d", runs.Load())
	}
}

func TestFormAfterRuntimeRunReturnsToPage(t *testing.T) {
	var runs atomic.Int32
	w, ts, cmd := liveWorld(t, "s1", func(*Session, noteIn) (any, error) { runs.Add(1); return nil, nil })
	h := cmd.Form(w, func(*http.Request, *Session, noteIn) (string, error) { runs.Add(1); return "/notes", nil }, nil)
	id := w.mintSubmit("s1")
	postSubmit(t, ts, "s1", "note.add", id, "a")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, formPOSTReq(id, "http://example.com/inbox?x=1"))
	if runs.Load() != 1 || rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/inbox?x=1" {
		t.Fatalf("runs %d: %d %q", runs.Load(), rec.Code, rec.Header().Get("Location"))
	}
}

func TestFormRefusedInputReleasesID(t *testing.T) {
	w := NewWorld()
	var runs atomic.Int32
	h := NewCommand[noteIn]("note.add").Form(w, func(*http.Request, *Session, noteIn) (string, error) {
		if runs.Add(1) == 1 {
			return "", Invalid("title", "taken")
		}
		return "/", nil
	}, nil)
	id := w.mintSubmit("s1")
	for _, want := range []int{http.StatusBadRequest, http.StatusSeeOther} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, formPOSTReq(id, ""))
		if rec.Code != want {
			t.Fatalf("got %d want %d", rec.Code, want)
		}
	}
}
