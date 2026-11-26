package live

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHeartbeatIsSentOnAnIdleStream(t *testing.T) {
	w := NewWorld(WithHeartbeat(20*time.Millisecond, time.Second))
	ts := serveWorld(t, w)
	r := openStream(t, ts, "hb")
	defer r.Close()

	deadline := time.Now().Add(2 * time.Second)
	seenHB := false
	for time.Now().Before(deadline) {
		line, ok := r.next(t, time.Second)
		if !ok {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(line), ": hb") {
			seenHB = true
			break
		}
	}
	if !seenHB {
		t.Fatal("idle stream must carry SSE keepalive comments (proxy liveness + dead-client detection)")
	}
}

func TestVanishedClientSessionIsReaped(t *testing.T) {
	w := NewWorld(WithHeartbeat(20*time.Millisecond, 50*time.Millisecond), WithResumeGrace(30*time.Millisecond))
	var mu sync.Mutex
	disconnected := make(chan string, 4)
	w.OnDisconnect(func(s *Session) {
		mu.Lock()
		defer mu.Unlock()
		disconnected <- s.sid
	})

	ts := serveWorld(t, w)
	r := openStream(t, ts, "dead")
	_, _ = r.next(t, time.Second)
	_, _ = r.next(t, time.Second)
	r.Close()

	select {
	case sid := <-disconnected:
		if sid != "dead" {
			t.Fatalf("OnDisconnect sid = %q, want dead", sid)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("vanished client's session was never retired (heartbeat/deadline reaping failed)")
	}
}

func TestHeartbeatDoesNotDisturbLiveSession(t *testing.T) {
	w := NewWorld(WithHeartbeat(15*time.Millisecond, time.Second))
	c := NewCell(w, "tick", 0)
	ts := serveWorld(t, w)
	r := openStream(t, ts, "alive")
	defer r.Close()
	_, _ = r.next(t, time.Second)
	_, _ = r.next(t, time.Second)

	time.Sleep(80 * time.Millisecond)
	c.Set(9)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, ok := r.next(t, time.Second)
		if !ok {
			break
		}
		if strings.Contains(line, `"v":9`) {
			return
		}
	}
	t.Fatal("live session stopped receiving frames after heartbeats")
}
