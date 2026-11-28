package live

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func startLoop(t *testing.T, s *Session, w io.Writer) (*connState, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := newConn(cancel)
	prelude, _, _, _ := s.attach(c)
	done := make(chan error, 1)
	go func() {
		defer c.finish()
		done <- s.loop(ctx, c, prelude, w, func() {})
	}()
	return c, done
}

func swapEv(topic, html string) event {
	return event{kind: KindSwap, topic: topic, body: []byte(`{"html":"` + html + `"}`)}
}

func cachedHTML(cached []event) string {
	var b strings.Builder
	for _, ev := range cached {
		b.WriteString(ev.topic + "=" + string(ev.body) + ";")
	}
	return b.String()
}

func TestTakeoverKeepsSwapsInTheCoalesceWindow(t *testing.T) {
	w := NewWorld(WithCoalesceWindow(time.Hour))
	s := &Session{world: w, sid: "h1", allTopics: true}
	c1, done := startLoop(t, s, io.Discard)

	s.offer(swapEv("t", "A"))
	waitFor(t, "loop to pull the swap", func() bool { return len(c1.inbox) == 0 })

	c2 := newConn(func() {})
	cached, stale, resumed, _ := s.attach(c2)
	if stale || !resumed {
		t.Fatalf("stale=%v resumed=%v", stale, resumed)
	}
	if len(cached) != 1 || !strings.Contains(string(cached[0].body), "A") {
		t.Fatalf("swap pulled into the old batch was lost: %q", cachedHTML(cached))
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTakeoverKeepsSwapsOfAFailedWrite(t *testing.T) {
	w := NewWorld(WithCoalesceWindow(time.Millisecond))
	s := &Session{world: w, sid: "h2", allTopics: true}
	var armed atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	wr := writerFunc(func(p []byte) (int, error) {
		if armed.Load() && strings.Contains(string(p), `"kind":"swap"`) {
			close(entered)
			<-release
			return 0, errors.New("broken pipe")
		}
		return len(p), nil
	})
	c1, done := startLoop(t, s, wr)
	waitFor(t, "initial frame", func() bool { return s.Attached() })
	armed.Store(true)
	s.offer(swapEv("t", "B"))
	<-entered

	type res struct {
		cached []event
		stale  bool
	}
	got := make(chan res, 1)
	c2 := newConn(func() {})
	go func() {
		cached, stale, _, _ := s.attach(c2)
		got <- res{cached, stale}
	}()
	waitFor(t, "attach to start replacing", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.replacing == c1
	})
	close(release)
	r := <-got
	if r.stale || len(r.cached) != 1 || !strings.Contains(string(r.cached[0].body), "B") {
		t.Fatalf("failed-write swap lost: stale=%v %q", r.stale, cachedHTML(r.cached))
	}
	if err := <-done; err == nil {
		t.Fatal("loop should report the write error")
	}
}

func TestTakeoverWaitIsBounded(t *testing.T) {
	old := attachWait
	attachWait = 20 * time.Millisecond
	t.Cleanup(func() { attachWait = old })

	w := NewWorld()
	s := &Session{world: w, sid: "h3", allTopics: true}
	s.attach(newConn(func() {}))
	_, stale, resumed, _ := s.attach(newConn(func() {}))
	if !stale || !resumed {
		t.Fatalf("stale=%v resumed=%v, want a stale resume", stale, resumed)
	}
}

func TestTakeoverKeepsLatestPerTopic(t *testing.T) {
	w := NewWorld(WithCoalesceWindow(time.Hour))
	s := &Session{world: w, sid: "h4", allTopics: true}
	c1, _ := startLoop(t, s, io.Discard)
	s.offer(swapEv("t", "old"))
	waitFor(t, "loop to pull", func() bool { return len(c1.inbox) == 0 })
	s.offer(swapEv("t", "new"))
	cached, _, _, _ := s.attach(newConn(func() {}))
	if len(cached) != 1 || !strings.Contains(string(cached[0].body), "new") {
		t.Fatalf("want only the newest swap, got %q", cachedHTML(cached))
	}
}
