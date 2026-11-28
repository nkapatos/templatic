package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/livevalue/components"
)

func TestFillCapsAndEmpty(t *testing.T) {
	w := live.NewWorld()
	mux := http.NewServeMux()
	level := newTank(w, mux)
	s := livetest.New(t, w)

	for range 2 {
		if rec := s.PostForm(mux, "/fill", url.Values{"amount": {"50"}}); rec.Code != http.StatusNoContent {
			t.Fatalf("fill: %d", rec.Code)
		}
	}
	if got := level.Get(); got != components.Capacity {
		t.Fatalf("level = %d, want %d", got, components.Capacity)
	}
	if rec := s.PostForm(mux, "/empty", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("empty: %d", rec.Code)
	}
	var got int
	if !s.Cell("tank.level", &got) || got != 0 {
		t.Fatalf("pushed level = %d, want 0", got)
	}
}

func TestFillRefusesZero(t *testing.T) {
	w := live.NewWorld()
	mux := http.NewServeMux()
	newTank(w, mux)
	s := livetest.New(t, w)
	if rec := s.PostForm(mux, "/fill", url.Values{"amount": {"0"}}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"amount":`) {
		t.Fatalf("%d %s, want 400 with an error on amount", rec.Code, rec.Body)
	}
}
