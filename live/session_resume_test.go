package live

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a-h/templ"
)

func htmlComp(s string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}

type lifecycle struct {
	mu  sync.Mutex
	log []string
}

func (l *lifecycle) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log = append(l.log, s)
}

func (l *lifecycle) get() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.log, ",")
}

func trackLifecycle(w *World) *lifecycle {
	l := &lifecycle{}
	w.OnConnect(func(*Session) { l.add("connect") })
	w.OnResume(func(*Session) { l.add("resume") })
	w.OnDisconnect(func(*Session) { l.add("disconnect") })
	return l
}

func sessionOf(w *World, sid string) *Session {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.sessions[sid]
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func drop(t *testing.T, w *World, sid string, st *stream) {
	t.Helper()
	st.Close()
	s := sessionOf(w, sid)
	if s == nil {
		return
	}
	waitFor(t, "session to detach", func() bool { return !s.Attached() })
}

func readUntilReady(t *testing.T, st *stream) []string {
	t.Helper()
	var out []string
	for {
		f, ok := st.next(t, 2*time.Second)
		if !ok {
			t.Fatalf("ready never arrived; got %q", out)
		}
		if f == "" || strings.Contains(f, `"kind":"resync"`) {
			t.Fatalf("stream ended (EOF/resync) before ready; got %q", out)
		}
		out = append(out, f)
		if strings.Contains(f, `"kind":"ready"`) {
			return out
		}
	}
}

func count(frames []string, sub string) int {
	n := 0
	for _, f := range frames {
		if strings.Contains(f, sub) {
			n++
		}
	}
	return n
}

var reconnectHeader = map[string]string{"Last-Event-ID": "3"}

func TestResumeKeepsSessionCellsAndFiresOnResume(t *testing.T) {
	w := NewWorld(WithResumeGrace(5 * time.Second))
	life := trackLifecycle(w)
	w.OnConnect(func(s *Session) { SessionCell(w, s, "note.open", "n1") })
	ts := serveWorld(t, w)

	a := openStream(t, ts, "p1")
	readUntilReady(t, a)
	cell := SessionCell(w, sessionOf(w, "p1"), "note.open", "ignored")
	cell.Set("n2")
	drop(t, w, "p1", a)
	cell.Set("n3")

	b := openStreamWithHeaders(t, ts, "p1", reconnectHeader)
	frames := readUntilReady(t, b)
	if count(frames, `"topic":"note.open","v":"n3"`) != 1 {
		t.Fatalf("session cell set while torn missing from the resumed frame: %q", frames)
	}
	if got := life.get(); got != "connect,resume" {
		t.Fatalf("lifecycle = %s, want connect,resume (OnDisconnect must wait for grace expiry)", got)
	}
}

func TestTornSwapsKeepLatestPerTopic(t *testing.T) {
	w := NewWorld(WithResumeGrace(5 * time.Second))
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p2")
	readUntilReady(t, a)
	s := sessionOf(w, "p2")
	drop(t, w, "p2", a)

	for _, v := range []string{"one", "two", "three"} {
		if err := w.Swap(s, "panel", htmlComp("<p>"+v+"</p>")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Swap(s, "other", htmlComp("<p>o</p>")); err != nil {
		t.Fatal(err)
	}

	b := openStreamWithHeaders(t, ts, "p2", reconnectHeader)
	frames := readUntilReady(t, b)
	if n := count(frames, `"kind":"swap","topic":"panel"`); n != 1 {
		t.Fatalf("want exactly one panel swap, got %d: %q", n, frames)
	}
	if count(frames, "three") != 1 || count(frames, "one")+count(frames, "two") != 0 {
		t.Fatalf("only the latest swap may survive: %q", frames)
	}
	if count(frames, `"topic":"other"`) != 1 {
		t.Fatalf("second topic's swap lost: %q", frames)
	}
	drop(t, w, "p2", b)
	c := openStreamWithHeaders(t, ts, "p2", reconnectHeader)
	if n := count(readUntilReady(t, c), `"kind":"swap"`); n != 0 {
		t.Fatalf("cache must be consumed by the reattach, replayed %d swaps", n)
	}
}

func TestTornWorldCellAndRegionArrive(t *testing.T) {
	w := NewWorld(WithResumeGrace(5 * time.Second))
	tick := NewCell(w, "tick", 0)
	w.Region("feed", func() templ.Component { return htmlComp("<p>feed</p>") })
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p3")
	readUntilReady(t, a)
	drop(t, w, "p3", a)
	tick.Set(7)
	if err := w.Redraw("feed"); err != nil {
		t.Fatal(err)
	}
	b := openStreamWithHeaders(t, ts, "p3", reconnectHeader)
	frames := readUntilReady(t, b)
	if count(frames, `"topic":"tick","v":7`) != 1 || count(frames, `"topic":"feed"`) != 1 {
		t.Fatalf("world cell/region set while torn missing: %q", frames)
	}
}

func TestTornStreamAppendCatchesUp(t *testing.T) {
	w := NewWorld(WithResumeGrace(5 * time.Second))
	w.Stream("drops.list", func() templ.Component { return htmlComp("<i>truth</i>") })
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p4")
	readUntilReady(t, a)
	drop(t, w, "p4", a)
	if err := w.Append("drops.list", "d1", htmlComp("x")); err != nil {
		t.Fatal(err)
	}
	b := openStreamWithHeaders(t, ts, "p4", reconnectHeader)
	readUntilReady(t, b)
	f, ok := b.next(t, 2*time.Second)
	if !ok || !strings.Contains(f, `"kind":"swap"`) || !strings.Contains(f, "truth") {
		t.Fatalf("stream catch-up swap missing after reattach: %q ok=%v", f, ok)
	}
}

func TestGraceExpiryRetiresAndResyncs(t *testing.T) {
	w := NewWorld(WithResumeGrace(60 * time.Millisecond))
	life := trackLifecycle(w)
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p5")
	readUntilReady(t, a)
	drop(t, w, "p5", a)
	if got := life.get(); got != "connect" {
		t.Fatalf("OnDisconnect fired on the drop, not at expiry: %s", got)
	}
	waitFor(t, "grace expiry", func() bool { return sessionOf(w, "p5") == nil })
	waitFor(t, "OnDisconnect", func() bool { return life.get() == "connect,disconnect" })

	b := openStreamWithHeaders(t, ts, "p5", reconnectHeader)
	f, ok := b.next(t, 2*time.Second)
	if !ok || !strings.Contains(f, `"kind":"resync"`) {
		t.Fatalf("late reconnect must resync, got %q ok=%v", f, ok)
	}
	if sessionOf(w, "p5") != nil || life.get() != "connect,disconnect" {
		t.Fatalf("a resync answer must not create a session: %s", life.get())
	}
}

func TestReattachCancelsGraceTimer(t *testing.T) {
	w := NewWorld(WithResumeGrace(80 * time.Millisecond))
	life := trackLifecycle(w)
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p6")
	readUntilReady(t, a)
	drop(t, w, "p6", a)
	b := openStreamWithHeaders(t, ts, "p6", reconnectHeader)
	readUntilReady(t, b)
	time.Sleep(250 * time.Millisecond)
	if sessionOf(w, "p6") == nil || !sessionOf(w, "p6").Attached() {
		t.Fatalf("reattached session was retired by the stale timer (%s)", life.get())
	}
	if strings.Contains(life.get(), "disconnect") {
		t.Fatalf("OnDisconnect fired for a live session: %s", life.get())
	}
}

func TestZeroGraceReconnectResyncs(t *testing.T) {
	w := NewWorld(WithResumeGrace(0))
	life := trackLifecycle(w)
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p7")
	readUntilReady(t, a)
	a.Close()
	waitFor(t, "immediate retirement", func() bool { return life.get() == "connect,disconnect" })
	b := openStreamWithHeaders(t, ts, "p7", reconnectHeader)
	if f, ok := b.next(t, 2*time.Second); !ok || !strings.Contains(f, `"kind":"resync"`) {
		t.Fatalf("grace 0 reconnect must resync: %q", f)
	}
	if life.get() != "connect,disconnect" {
		t.Fatalf("reconnect created a session: %s", life.get())
	}
}

func TestLiveConnectionIsTakenOver(t *testing.T) {
	w := NewWorld(WithResumeGrace(5 * time.Second))
	life := trackLifecycle(w)
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p8")
	readUntilReady(t, a)
	s := sessionOf(w, "p8")
	b := openStreamWithHeaders(t, ts, "p8", reconnectHeader)
	readUntilReady(t, b)
	if sessionOf(w, "p8") != s {
		t.Fatal("takeover must keep the same session")
	}
	if got := life.get(); got != "connect,resume" {
		t.Fatalf("lifecycle = %s", got)
	}
	c := NewCell(w, "after", 0)
	c.Set(1)
	for {
		f, ok := b.next(t, 2*time.Second)
		if !ok {
			t.Fatal("new connection never received the publish")
		}
		if strings.Contains(f, `"topic":"after"`) {
			break
		}
	}
}

func TestDetachedCacheIsBoundedByTopics(t *testing.T) {
	w := NewWorld(WithResumeGrace(5 * time.Second))
	life := trackLifecycle(w)
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p9")
	readUntilReady(t, a)
	s := sessionOf(w, "p9")
	drop(t, w, "p9", a)
	for i := 0; i < maxDetachedTopics; i++ {
		_ = w.Swap(s, fmt.Sprintf("t%d", i), htmlComp("x"))
	}
	s.mu.Lock()
	n, stale := len(s.cache), s.stale
	s.mu.Unlock()
	if n != maxDetachedTopics || stale {
		t.Fatalf("at the bound: len=%d stale=%v", n, stale)
	}
	_ = w.Swap(s, "t0", htmlComp("y"))
	s.mu.Lock()
	n, stale = len(s.cache), s.stale
	s.mu.Unlock()
	if n != maxDetachedTopics || stale {
		t.Fatalf("overwrite must not grow the cache: len=%d stale=%v", n, stale)
	}
	_ = w.Swap(s, "one-too-many", htmlComp("x"))
	b := openStreamWithHeaders(t, ts, "p9", reconnectHeader)
	f, ok := b.next(t, 2*time.Second)
	if !ok || !strings.Contains(f, `"kind":"resync"`) {
		t.Fatalf("stale reattach must resync, got %q", f)
	}
	waitFor(t, "stale session retired", func() bool { return life.get() == "connect,disconnect" })
}

func TestDetachedOfferDropsNonSwaps(t *testing.T) {
	w := NewWorld(WithResumeGrace(5 * time.Second))
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p10")
	readUntilReady(t, a)
	s := sessionOf(w, "p10")
	drop(t, w, "p10", a)
	for i := 0; i < 5000; i++ {
		s.offer(event{kind: KindCell, topic: "tick", body: []byte(`{"v":1}`)})
		s.offer(event{kind: KindAppend, topic: "list", body: []byte(`{"items":[]}`)})
	}
	s.mu.Lock()
	n := len(s.cache)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("detached cache holds %d non-swap entries", n)
	}
}

func TestResumeIsRaceFree(t *testing.T) {
	w := NewWorld(WithResumeGrace(time.Second))
	life := trackLifecycle(w)
	ts := serveWorld(t, w)
	a := openStream(t, ts, "p11")
	readUntilReady(t, a)
	s := sessionOf(w, "p11")
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = w.Swap(s, "panel", htmlComp(fmt.Sprint(i)))
			// Paced: a flood would overflow the 1024-event inbox and retire the session.
			time.Sleep(100 * time.Microsecond)
		}
	}()
	cur := a
	for i := 0; i < 5; i++ {
		drop(t, w, "p11", cur)
		cur = openStreamWithHeaders(t, ts, "p11", reconnectHeader)
		readUntilReady(t, cur)
	}
	close(stop)
	wg.Wait()
	if strings.Count(life.get(), "connect,") != 1 || strings.Contains(life.get(), "disconnect") {
		t.Fatalf("lifecycle = %s", life.get())
	}
}
