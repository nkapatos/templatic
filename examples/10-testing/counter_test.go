package main

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/testing/components"
)

func TestAddReachesEveryPage(t *testing.T) {
	w := live.NewWorld()
	newCounter(w)
	a, b := livetest.New(t, w), livetest.New(t, w)

	if _, err := livetest.Call(a, components.Add, components.AddIn{N: 2}); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]*livetest.Session{"a": a, "b": b} {
		var got int
		if !s.Cell(components.CountTopic, &got) || got != 2 {
			t.Errorf("%s: count = %d, want 2", name, got)
		}
		if items := s.Appends(components.Log.Topic()); len(items) != 1 || !strings.Contains(items[0], "+2 → 2") {
			t.Errorf("%s: log appends = %q, want one +2 → 2", name, items)
		}
	}
}

func TestAddSwapsOnlyTheCallersTally(t *testing.T) {
	w := live.NewWorld()
	newCounter(w)
	a, b := livetest.New(t, w), livetest.New(t, w)

	for _, n := range []int{1, 3} {
		if _, err := livetest.Call(a, components.Add, components.AddIn{N: n}); err != nil {
			t.Fatal(err)
		}
	}
	if sw := a.Swaps(components.TallyTopic); len(sw) != 2 {
		t.Fatalf("a: %d tally swaps, want 2", len(sw))
	}
	a.WaitSwap(components.TallyTopic, "added 2 times, last 3", time.Second)
	if html, ok := b.LastSwap(components.TallyTopic); ok {
		t.Fatalf("b got a tally swap: %s", html)
	}
}

func TestAddAsAFormSubmit(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	s := livetest.New(t, w)

	// Form fields decode by the input's field types: "4" becomes 4.
	if _, err := livetest.Submit(s, components.Add, url.Values{"n": {"4"}}); err != nil {
		t.Fatal(err)
	}
	if got := c.count.Get(); got != 4 {
		t.Fatalf("count = %d, want 4", got)
	}
}

func TestAddRefusesABadStep(t *testing.T) {
	w := live.NewWorld()
	newCounter(w)
	s := livetest.New(t, w)

	refused := func(what string, err error) {
		t.Helper()
		if errs := live.InputErrors(err); len(errs) != 1 || errs[0].Field != "n" {
			t.Errorf("%s: err = %v, want one error on n", what, err)
		}
	}
	_, err := livetest.Call(s, components.Add, components.AddIn{N: 0})
	refused("0", err)
	_, err = livetest.Call(s, components.Add, components.AddIn{N: 500})
	refused("500", err)
	_, err = livetest.Submit(s, components.Add, url.Values{"n": {"two"}})
	refused("two", err)

	if ev := s.Events(); len(ev) != 0 {
		t.Fatalf("a refused add sent %d events, want none", len(ev))
	}
}

func TestTickAppendsFromAGoroutine(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	s := livetest.New(t, w)

	// No command behind it: the append arrives on its own, so wait for it.
	go c.tick(t.Context(), 5*time.Millisecond)
	s.WaitAppend(components.Log.Topic(), "server clock", time.Second)
}

func TestTheLogKeepsItsLastEntries(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	for i := range keepEntries + 3 {
		if err := c.add(i + 1); err != nil {
			t.Fatal(err)
		}
	}
	got := c.entries()
	if len(got) != keepEntries || !strings.HasPrefix(got[0].Text, "+4 ") {
		t.Fatalf("kept %d entries from %q, want %d from +4", len(got), got[0].Text, keepEntries)
	}
}

func TestAPageTallyEndsWithItsSession(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	// A session is retired when its test ends, so a subtest scopes one page.
	t.Run("page", func(t *testing.T) {
		s := livetest.New(t, w)
		if _, err := livetest.Call(s, components.Add, components.AddIn{N: 1}); err != nil {
			t.Fatal(err)
		}
	})
	c.mu.Lock()
	n := len(c.pages)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d tallies kept after the page left, want 0", n)
	}
}
