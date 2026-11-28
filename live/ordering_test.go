package live

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
)

func text(s string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}

func TestLateCellPublishDoesNotOverwriteNewerValue(t *testing.T) {
	w := NewWorld()
	price := NewCell(w, "price", 1)
	ts := serveWorld(t, w)
	r := openStream(t, ts, "s1")
	defer r.Close()
	r.discardUntilReady(t)

	price.Set(5) // version 2
	if f, ok := r.next(t, 2*time.Second); !ok || !strings.Contains(f, `"v":5`) {
		t.Fatalf("got %q", f)
	}
	price.publishValue(4, 1) // a racing Set(4) whose publish lands last, in a batch of its own
	time.Sleep(50 * time.Millisecond)
	price.Set(6)
	if f, ok := r.next(t, 2*time.Second); !ok || !strings.Contains(f, `"v":6`) {
		t.Fatalf("older publish reached the page: %q", f)
	}
}

func TestRefreshLosesToSwapMadeWhileReadingItsModel(t *testing.T) {
	w := NewWorld()
	region := DeclareSessionRegion[string]("view").Renders(func(m string) templ.Component { return text(m) })
	ts := serveWorld(t, w)
	r := openStream(t, ts, "s1")
	defer r.Close()
	r.discardUntilReady(t)

	err := region.Refresh(w, func(s *Session) (string, bool) {
		// a command swaps newer state between Refresh reading the model and swapping it
		if err := region.Swap(s, "new"); err != nil {
			t.Fatal(err)
		}
		return "old", true
	})
	if err != nil {
		t.Fatal(err)
	}
	f, ok := r.next(t, 2*time.Second)
	if !ok || !strings.Contains(f, `"html":"new"`) {
		t.Fatalf("got %q", f)
	}
	if f, ok := r.next(t, 200*time.Millisecond); ok && f != "" {
		t.Fatalf("the older render followed: %q", f)
	}
}

func TestStalledWriterHoldsOneSlotPerCellTopic(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "s1")
	a, b := NewCell(w, "a", 0), NewCell(w, "b", 0)
	for i := range 4 * inboxSize {
		a.Set(i)
		b.Set(i)
	}
	if s.conn.overflow.Load() {
		t.Fatal("cell writes overflowed the inbox")
	}
	if n := len(s.conn.inbox); n != 2 {
		t.Fatalf("inbox holds %d events, want one per topic", n)
	}
	s.mu.Lock()
	got := s.conn.takeLocked(<-s.conn.inbox)
	s.mu.Unlock()
	if !strings.Contains(string(got.body), `"v":4095`) {
		t.Fatalf("slot carries %s, want the latest value", got.body)
	}
}
