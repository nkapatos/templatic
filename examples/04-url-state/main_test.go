package main

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/urlstate/components"
)

func TestNavigateSwapsOnlyItsSession(t *testing.T) {
	w := live.NewWorld()
	components.Navigate.Handle(w, navigate)
	a, b := livetest.New(t, w), livetest.New(t, w)

	res, err := livetest.Call(a, components.Navigate, components.View{Tab: "done", Open: "7"})
	if err != nil {
		t.Fatal(err)
	}
	if push, ok := res.(live.Push); !ok || push.URL() != "/?open=7&tab=done" {
		t.Fatalf("result = %#v, want live.At(/?open=7&tab=done)", res)
	}
	a.WaitSwap(components.InboxTopic, "Rotate the deploy keys", time.Second)
	if _, ok := b.LastSwap(components.InboxTopic); ok {
		t.Fatal("the other session got a swap")
	}
}

func TestViewURLRoundTrip(t *testing.T) {
	for _, v := range []components.View{
		{Tab: "open"},
		{Tab: "done", Q: "keys & locks", Open: "7"},
		{Tab: "open", Open: "1"},
	} {
		u, err := url.Parse(v.URL())
		if err != nil {
			t.Fatal(err)
		}
		if got := components.ParseView(u.Query()); got != v {
			t.Errorf("ParseView(%q) = %+v, want %+v", v.URL(), got, v)
		}
	}
}

func TestParseViewFallsBack(t *testing.T) {
	q := url.Values{"tab": {"bogus"}, "open": {"99"}, "q": {"  " + strings.Repeat("x", 100)}}
	got := components.ParseView(q)
	if got.Tab != "open" || got.Open != "" || len(got.Q) != 64 {
		t.Fatalf("ParseView = %+v", got)
	}
}

func TestRefiningASearchReplacesTheEntry(t *testing.T) {
	w := live.NewWorld()
	components.Navigate.Handle(w, navigate)
	s := livetest.New(t, w)

	for _, step := range []struct {
		view    components.View
		replace bool
	}{
		{components.View{Tab: "open", Q: "ke"}, false},  // the first search is a step back
		{components.View{Tab: "open", Q: "keys"}, true}, // typing on refines it
		{components.View{Tab: "done", Q: "keys"}, false},
	} {
		res, err := livetest.Call(s, components.Navigate, step.view)
		if err != nil {
			t.Fatal(err)
		}
		if push := res.(live.Push); push.Replaces() != step.replace {
			t.Errorf("%+v: replaces = %v, want %v", step.view, push.Replaces(), step.replace)
		}
	}
}
