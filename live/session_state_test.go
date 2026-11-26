package live

import (
	"context"
	"encoding/json/jsontext"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
)

func TestSessionCellPublishesToOwnerOnly(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	a := openStream(t, ts, "sa")
	defer a.Close()
	b := openStream(t, ts, "sb")
	defer b.Close()
	a.discardUntilReady(t)
	b.discardUntilReady(t)

	w.Command("note.open", func(s *Session, args Args) (any, error) {
		var in struct {
			ID string `json:"id"`
		}
		if err := args.Into(&in); err != nil {
			return nil, err
		}
		SessionCell(w, s, "note.open", in.ID).Set(in.ID)
		return nil, nil
	})

	postCommand(t, ts, "sa", "note.open", jsontext.Value(`{"id":"arch-notes"}`))

	frame, ok := a.next(t, 2*time.Second)
	if !ok || !strings.Contains(frame, `"topic":"note.open"`) || !strings.Contains(frame, `"v":"arch-notes"`) {
		t.Fatalf("owner missing its cell update: %q ok=%v", frame, ok)
	}
	if other, ok := b.next(t, 300*time.Millisecond); ok {
		t.Fatalf("leaked across sessions: %q", other)
	}
}

func TestSessionCellDeclareReturnsExisting(t *testing.T) {
	w := NewWorld()
	sa := newTestSession(t, w, "sa")
	sb := newTestSession(t, w, "sb")

	first := SessionCell(w, sa, "note.open", "arch-notes")
	again := SessionCell(w, sa, "note.open", "other-note")
	if first != again {
		t.Fatal("redeclare returned a new cell")
	}
	if got := first.Get(); got != "arch-notes" {
		t.Fatalf("redeclare reset the value: %q", got)
	}

	other := SessionCell(w, sb, "note.open", "data-model")
	if other == first {
		t.Fatal("sessions share a cell handle")
	}
	first.Set("api-design")
	if got := other.Get(); got != "data-model" {
		t.Fatalf("cross-session write leaked: %q", got)
	}

	select {
	case ev := <-sa.conn.inbox:
		if ev.topic != "note.open" {
			t.Fatalf("unexpected event topic: %q", ev.topic)
		}
	default:
		t.Fatal("session cell event never reached its owner")
	}
	select {
	case ev := <-sb.conn.inbox:
		t.Fatalf("session cell event leaked to sb: %v", ev.topic)
	default:
	}
}

func TestSessionCellRidesInitialSnapshot(t *testing.T) {
	w := NewWorld()
	w.OnConnect(func(s *Session) {
		SessionCell(w, s, "note.open", "arch-notes")
	})

	ts := serveWorld(t, w)
	r := openStream(t, ts, "sc")
	defer r.Close()

	frame, ok := r.next(t, 2*time.Second)
	if !ok || !strings.Contains(frame, `"topic":"note.open"`) || !strings.Contains(frame, `"v":"arch-notes"`) {
		t.Fatalf("initial frame missing session cell: %q ok=%v", frame, ok)
	}
	if ready, _ := r.next(t, 2*time.Second); !strings.Contains(ready, `"kind":"ready"`) {
		t.Fatalf("ready frame missing: %q", ready)
	}
}

func TestSwapSendsToOneSession(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	a := openStream(t, ts, "sd")
	defer a.Close()
	b := openStream(t, ts, "se")
	defer b.Close()
	a.discardUntilReady(t)
	b.discardUntilReady(t)

	markup := templ.ComponentFunc(func(_ context.Context, _ io.Writer) error { return nil })
	w.Command("panel.swap", func(s *Session, args Args) (any, error) {
		return nil, w.Swap(s, "workspace.surface", markup)
	})

	postCommand(t, ts, "sd", "panel.swap", nil)

	frame, ok := a.next(t, 2*time.Second)
	if !ok || !strings.Contains(frame, `"kind":"swap"`) || !strings.Contains(frame, `"topic":"workspace.surface"`) {
		t.Fatalf("target session missing its swap: %q ok=%v", frame, ok)
	}
	if other, ok := b.next(t, 300*time.Millisecond); ok {
		t.Fatalf("swap leaked across sessions: %q", other)
	}
}

func TestSwapRejectsNilAndValidatesTopic(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "sf")
	if err := w.Swap(s, "panel", nil); err == nil {
		t.Fatal("expected error for nil component")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on malformed topic")
		}
	}()
	markup := templ.ComponentFunc(func(_ context.Context, _ io.Writer) error { return nil })
	_ = w.Swap(s, "bad topic!", markup)
}

func newTestSession(t *testing.T, w *World, sid string) *Session {
	s := &Session{sid: sid, world: w, conn: newConn(func() {})}
	w.mu.Lock()
	w.sessions[sid] = s
	w.mu.Unlock()
	t.Cleanup(func() {
		w.mu.Lock()
		delete(w.sessions, sid)
		w.mu.Unlock()
	})
	return s
}
