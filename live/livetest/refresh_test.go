package livetest_test

import (
	"testing"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"
)

func TestRefreshRendersEachSessionFromItsOwnState(t *testing.T) {
	page := live.DeclareSessionCell("t.page", 1)
	table := live.DeclareSessionRegion[int]("t.table").Renders(func(p int) templ.Component {
		return text("page " + string(rune('0'+p)))
	})
	w := live.NewWorld()
	a, b, c := livetest.New(t, w), livetest.New(t, w), livetest.New(t, w)
	page.Set(a.Session, 2)
	page.Set(b.Session, 3)
	if err := table.Refresh(w, func(s *live.Session) (int, bool) {
		return page.Get(s), s != c.Session
	}); err != nil {
		t.Fatal(err)
	}
	if h, _ := a.LastSwap("t.table"); h != "page 2" {
		t.Fatalf("a: %q", h)
	}
	if h, _ := b.LastSwap("t.table"); h != "page 3" {
		t.Fatalf("b: %q", h)
	}
	if len(c.Swaps("t.table")) != 0 {
		t.Fatal("skipped session swapped")
	}
}
