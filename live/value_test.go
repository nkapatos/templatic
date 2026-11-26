package live

import (
	"sync"
	"testing"
	"time"
)

func TestSessionValueDeclareOrReturnAndTyped(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "v1")
	a := SessionValue(w, s, "assistant.dismissed", []string{"x"})
	b := SessionValue(w, s, "assistant.dismissed", []string{"ignored"})
	if a != b || len(b.Get()) != 1 {
		t.Fatal("declare must return the existing value, initial ignored")
	}
	a.Update(func(old []string) []string { return append(old, "y") })
	if got := b.Get(); len(got) != 2 || got[1] != "y" {
		t.Fatalf("Update lost: %v", got)
	}
	other := newTestSession(t, w, "v2")
	if c := SessionValue(w, other, "assistant.dismissed", []string{}); len(c.Get()) != 0 {
		t.Fatal("values are per session")
	}
}

func TestSessionValueNeverPublishes(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	st := openStream(t, ts, "v3")
	readUntilReady(t, st)
	s := sessionOf(w, "v3")
	v := SessionValue(w, s, "secret.key", "s3cr3t")
	v.Set("rotated")
	v.Update(func(string) string { return "again" })
	if f, ok := st.next(t, 200*time.Millisecond); ok {
		t.Fatalf("SessionValue.Set put a frame on the wire: %q", f)
	}
	s.mu.Lock()
	queued := len(s.conn.inbox)
	s.mu.Unlock()
	if queued != 0 {
		t.Fatal("SessionValue.Set queued an event")
	}
	if got := s.snapshot(); len(got) != 0 {
		t.Fatalf("values must not be enumerated in the initial frame: %v", got)
	}
}

func TestSessionValueSurvivesReattach(t *testing.T) {
	w := NewWorld(WithResumeGrace(5 * time.Second))
	ts := serveWorld(t, w)
	a := openStream(t, ts, "v4")
	readUntilReady(t, a)
	s := sessionOf(w, "v4")
	SessionValue(w, s, "dismissed", 0).Set(7)
	drop(t, w, "v4", a)
	b := openStreamWithHeaders(t, ts, "v4", reconnectHeader)
	frames := readUntilReady(t, b)
	if count(frames, `"dismissed"`) != 0 {
		t.Fatalf("value leaked into the resumed frame: %q", frames)
	}
	if got := SessionValue(w, sessionOf(w, "v4"), "dismissed", -1).Get(); got != 7 {
		t.Fatalf("value after reattach = %d, want 7", got)
	}
}

func TestSessionValueIsRaceFree(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "v5")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := SessionValue(w, s, "n", 0)
			for j := 0; j < 200; j++ {
				v.Update(func(o int) int { return o + 1 })
			}
		}()
	}
	wg.Wait()
	if got := SessionValue(w, s, "n", 0).Get(); got != 1600 {
		t.Fatalf("lost updates: %d", got)
	}
}

func TestSessionValueValidatesInput(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "v6")
	for name, fn := range map[string]func(){
		"bad key":     func() { SessionValue(w, s, "bad key!", 1) },
		"nil session": func() { SessionValue(w, nil, "k", 1) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: expected panic", name)
				}
			}()
			fn()
		}()
	}
}
