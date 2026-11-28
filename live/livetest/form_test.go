package livetest_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"
)

func TestPostFormDrivesAFormHandlerAsTheRuntime(t *testing.T) {
	type noteIn struct {
		Title string `json:"title"`
	}
	w := live.NewWorld()
	count := live.NewCell(w, "count", 0)
	var seen *live.Session
	h := live.NewCommand[noteIn]("note.add").Form(w, func(_ *http.Request, s *live.Session, in noteIn) (string, error) {
		if in.Title == "" {
			return "", live.Invalid("title", "required")
		}
		seen = s
		count.Set(1)
		return "/", nil
	}, nil)
	s := livetest.New(t, w)
	if rec := s.PostForm(h, "/notes", url.Values{"title": {"a"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	var n int
	if seen == nil || seen.SID() != s.SID() || !s.Cell("count", &n) || n != 1 {
		t.Fatal("the handler ran on the session and its update was recorded")
	}
	if rec := s.PostForm(h, "/notes", url.Values{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("refused input: %d", rec.Code)
	}
}

func TestSessionURLFollowsACommandsAt(t *testing.T) {
	w := live.NewWorld()
	w.Command("go", func(*live.Session, live.Args) (any, error) { return live.At("/inbox?tab=done"), nil })
	s := livetest.New(t, w)
	if _, err := s.Do("go", nil); err != nil {
		t.Fatal(err)
	}
	if u := s.URL(); u.Path != "/inbox" || u.RawQuery != "tab=done" {
		t.Fatalf("URL() = %v, want /inbox?tab=done", u)
	}
}
