package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/testing/components"
)

// add submits the page's add form with n, as the runtime posts it, and fails
// the test unless it is accepted.
func add(t *testing.T, w *live.World, s *livetest.Session, c *counter, n string) {
	t.Helper()
	if rec := s.PostForm(c.addForm(w, nil), "/add", url.Values{"n": {n}}); rec.Code != http.StatusNoContent {
		t.Fatalf("add %s: %d %s", n, rec.Code, rec.Body)
	}
}

// shown connects a page that rendered the log as it stands.
func shown(t *testing.T, w *live.World, c *counter) *livetest.Session {
	s := livetest.New(t, w)
	s.Show(c.log.Topic(), c.log.Version())
	return s
}

func TestAddReachesEveryPage(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	a, b := shown(t, w, c), shown(t, w, c)

	add(t, w, a, c, "2")
	for name, s := range map[string]*livetest.Session{"a": a, "b": b} {
		var got int
		if !s.Cell(components.CountTopic, &got) || got != 2 {
			t.Errorf("%s: count = %d, want 2", name, got)
		}
		if ops := s.Ops(components.LogTopic); len(ops) != 1 || !strings.Contains(ops[0].HTML, "+2 → 2") {
			t.Errorf("%s: log ops = %v, want one +2 → 2", name, ops)
		}
	}
}

func TestAddSwapsOnlyTheCallersTally(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	a, b := livetest.New(t, w), livetest.New(t, w)

	for _, n := range []string{"1", "3"} {
		add(t, w, a, c, n)
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
	add(t, w, s, c, "4")
	if got := c.count.Get(); got != 4 {
		t.Fatalf("count = %d, want 4", got)
	}
}

func TestAddRefusesABadStep(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	s := livetest.New(t, w)
	h := c.addForm(w, nil)

	// A refusal answers 400 with the errors by field, as the runtime receives them.
	for _, n := range []string{"0", "500", "two"} {
		rec := s.PostForm(h, "/add", url.Values{"n": {n}})
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"fields":{"n":`) {
			t.Errorf("%s: %d %s, want 400 with an error on n", n, rec.Code, rec.Body)
		}
	}

	if ev := s.Events(); len(ev) != 0 {
		t.Fatalf("a refused add sent %d events, want none", len(ev))
	}
}

func TestTickLogsFromAGoroutine(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	s := shown(t, w, c)

	// No command behind it: the entry arrives on its own, so wait for it.
	go c.tick(t.Context(), 5*time.Millisecond)
	s.WaitOp(components.LogTopic, "server clock", time.Second)
}

func TestTheLogKeepsItsLastEntries(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	for i := range keepEntries + 3 {
		if err := c.add(i + 1); err != nil {
			t.Fatal(err)
		}
	}
	got := c.log.Items()
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
		add(t, w, s, c, "1")
	})
	c.mu.Lock()
	n := len(c.pages)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d tallies kept after the page left, want 0", n)
	}
}

func TestAResumedPageCatchesUpOnTheLog(t *testing.T) {
	w := live.NewWorld()
	c := newCounter(w)
	a, b := shown(t, w, c), shown(t, w, c)

	// a loses its connection; b adds while a is away, which a does not see.
	a.Drop()
	add(t, w, b, c, "3")
	if ops := a.Ops(components.LogTopic); len(ops) != 0 {
		t.Fatalf("a got %d log ops while dropped, want 0", len(ops))
	}
	// On resume a is sent the entries after the version it last received.
	if !a.Resume() {
		t.Fatal("a did not resume within the grace")
	}
	a.WaitOp(components.LogTopic, "+3 → 3", time.Second)
}
