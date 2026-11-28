package live

import "testing"

func TestAttachRefusesARetiredSession(t *testing.T) {
	w := NewWorld()
	s := &Session{world: w, sid: "r1", allTopics: true}
	w.mu.Lock()
	w.sessions["r1"] = s
	w.mu.Unlock()
	s.retire()

	c := newConn(func() {})
	if _, _, _, retired := s.attach(c); !retired {
		t.Fatal("attach to a retired session must report it")
	}
	if s.Attached() {
		t.Fatal("attach to a retired session must not attach")
	}
}

func TestHandlerRetriesAfterRetiredSession(t *testing.T) {
	w := NewWorld()
	retiring := &Session{world: w, sid: "r2", allTopics: true}
	retiring.retired = true
	w.mu.Lock()
	w.sessions["r2"] = retiring
	w.mu.Unlock()
	if !retiring.isRetired() {
		t.Fatal("setup")
	}
	ts := serveWorld(t, w)
	st := openStream(t, ts, "r2")
	readUntilReady(t, st)
	if s := sessionOf(w, "r2"); s == retiring || !s.Attached() {
		t.Fatal("fresh connect must replace the retiring session")
	}
}
