package live

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestValidateTopicRejectsEmpty(t *testing.T) {
	// An empty topic must panic: the coalescer treats "" as a kind-level event.
	for _, bad := range []string{""} {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic on empty topic")
			}
		}()
		validateTopic(bad)
	}
}

func TestNewCellRejectsEmptyTopic(t *testing.T) {
	w := NewWorld()
	defer func() {
		if recover() == nil {
			t.Fatal("expected NewCell to reject an empty topic")
		}
	}()
	NewCell(w, "", int64(0))
}

func TestCoalescerKeepsHighestVersion(t *testing.T) {
	mk := func(version uint64, body string) event {
		return event{kind: KindCell, topic: "tick", version: version, body: []byte(body)}
	}
	out := coalescer{}
	out.add(mk(2, `{"v":"newest"}`))
	out.add(mk(1, `{"v":"stale"}`))
	out.add(mk(3, `{"v":"final"}`))

	events := out.events()
	if len(events) != 1 {
		t.Fatalf("one coalesced cell event expected, got %d", len(events))
	}
	if string(events[0].body) != `{"v":"final"}` {
		t.Fatalf("highest version must win, got %q", events[0].body)
	}

	fresh := coalescer{}
	fresh.add(mk(5, `{"v":"only"}`))
	if got := fresh.events(); len(got) != 1 || string(got[0].body) != `{"v":"only"}` {
		t.Fatalf("single offer must stay: %+v", got)
	}
}

func TestOverflowSendsResyncDeterministically(t *testing.T) {
	w := NewWorld()
	ctx, cancel := context.WithCancel(context.Background())
	c := &connState{inbox: make(chan event, 1), cancel: cancel, done: make(chan struct{})}
	s := &Session{sid: "overfull", world: w, conn: c}
	c.inbox <- event{kind: KindCell, topic: "tick", body: []byte(`{"v":1}`)}
	s.offer(event{kind: KindCell, topic: "tick", body: []byte(`{"v":2}`)})
	if !c.overflow.Load() {
		t.Fatal("setup: second offer must overflow the capacity-1 inbox")
	}

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- s.loop(ctx, c, nil, &buf, func() {}) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loop returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not exit after overflow cancel")
	}
	if !strings.Contains(buf.String(), `"kind":"resync"`) {
		t.Fatalf("overflow must emit resync, got %q", buf.String())
	}
}
