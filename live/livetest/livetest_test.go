package livetest_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"
)

type openIn struct {
	ID string `json:"id"`
}

func text(s string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}

var (
	open    = live.NewCommand[openIn]("note.open")
	opened  = live.DeclareSessionCell("note.opened", "")
	surface = live.DeclareSessionRegion[string]("note.surface").Renders(func(id string) templ.Component { return text("<p>" + id + "</p>") })
)

func world() (*live.World, *live.Cell[int]) {
	w := live.NewWorld()
	count := live.NewCell(w, "notes.count", 0)
	open.Handle(w, func(s *live.Session, in openIn) (any, error) {
		if in.ID == "" {
			return nil, live.Invalid("id", "required")
		}
		if in.ID == "missing" {
			return nil, live.NotFound("no such note")
		}
		opened.Set(s, in.ID)
		count.Update(func(n int) int { return n + 1 })
		if in.ID == "slow" {
			go func() { time.Sleep(5 * time.Millisecond); _ = w.Append("notes.log", "k1", text("late")) }()
		}
		return nil, surface.Swap(s, in.ID)
	})
	w.Stream("notes.log", func() templ.Component { return text("") })
	return w, count
}

func TestCallRecordsSwapsAndCells(t *testing.T) {
	w, _ := world()
	s := livetest.New(t, w)
	if _, err := livetest.Call(s, open, openIn{ID: "n1"}); err != nil {
		t.Fatal(err)
	}
	if h, ok := s.LastSwap("note.surface"); !ok || h != "<p>n1</p>" {
		t.Fatalf("swap = %q %v", h, ok)
	}
	var id string
	var n int
	if !s.Cell("note.opened", &id) || id != "n1" || !s.Cell("notes.count", &n) || n != 1 {
		t.Fatalf("cells: %q %d", id, n)
	}
	if opened.Get(s.Session) != "n1" {
		t.Fatal("typed handle on the session")
	}
	if s.Cell("nope", &id) {
		t.Fatal("unknown cell reported")
	}
	s.Reset()
	if len(s.Events()) != 0 {
		t.Fatal("reset kept events")
	}
}

func TestErrorsComeBackAsReturned(t *testing.T) {
	w, _ := world()
	s := livetest.New(t, w)
	var ie *live.InputError
	if _, err := s.Do("note.open", openIn{}); !errors.As(err, &ie) || ie.Field != "id" {
		t.Fatalf("input error: %v", err)
	}
	var se *live.StatusError
	if _, err := s.Do("note.open", openIn{ID: "missing"}); !errors.As(err, &se) || se.Code != 404 {
		t.Fatalf("status error: %v", err)
	}
	var me *live.MissingCommandError
	if _, err := s.Do("nope", nil); !errors.As(err, &me) {
		t.Fatalf("missing command: %v", err)
	}
	if len(s.Swaps("note.surface")) != 0 {
		t.Fatal("refused call swapped")
	}
}

func TestSessionsAreIsolatedAndBroadcastsReachAll(t *testing.T) {
	w, _ := world()
	a, b := livetest.New(t, w), livetest.New(t, w)
	if _, err := livetest.Call(a, open, openIn{ID: "slow"}); err != nil {
		t.Fatal(err)
	}
	if len(b.Swaps("note.surface")) != 0 {
		t.Fatal("private swap leaked to another session")
	}
	var n int
	if !b.Cell("notes.count", &n) || n != 1 {
		t.Fatalf("world cell on b = %d", n)
	}
	deadline := time.Now().Add(time.Second)
	for len(b.Appends("notes.log")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("append never reached b")
		}
		time.Sleep(time.Millisecond)
	}
	a.WaitSwap("note.surface", "slow", time.Second)
}
