package live

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// viewer is an in-memory session whose inbox a goroutine drains, as a connected page's stream loop would.
type viewer struct {
	s     *Session
	c     *connState
	bytes atomic.Int64
	done  chan struct{}
}

func attachViewer(w *World, sid string) *viewer {
	s := &Session{world: w, sid: sid}
	w.mu.Lock()
	w.sessions[sid] = s
	w.mu.Unlock()
	_, cancel := context.WithCancel(context.Background())
	c := newConn(cancel)
	s.attach(c)
	v := &viewer{s: s, c: c, done: make(chan struct{})}
	go func() {
		for {
			select {
			case ev := <-c.inbox:
				v.bytes.Add(int64(len(ev.body)))
			case <-v.done:
				return
			}
		}
	}()
	return v
}

func (v *viewer) stop() { close(v.done) }

func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func filled(w *World, topic string, items int) *Collection[row] {
	c := rows(w, topic).Limit(items)
	batch := make([]row, 0, 1000)
	for i := range items {
		batch = append(batch, row{ID: fmt.Sprint("r", i), Text: "an eighty byte row of log text, roughly what 17-tanstack measured per row"})
		if len(batch) == cap(batch) {
			_ = c.Upsert(batch...)
			batch = batch[:0]
		}
	}
	_ = c.Upsert(batch...)
	return c
}

// BenchmarkCollectionUpsert is one new row published to every subscribed viewer of a full collection: the cost that
// was an 836 KB cell frame per page in 17-tanstack.
func BenchmarkCollectionUpsert(b *testing.B) {
	for _, viewers := range []int{100, 1000, 2000} {
		for _, items := range []int{1000, 10000} {
			b.Run(fmt.Sprintf("viewers=%d/items=%d", viewers, items), func(b *testing.B) {
				w := NewWorld()
				c := filled(w, "log", items)
				vs := make([]*viewer, viewers)
				for i := range vs {
					vs[i] = attachViewer(w, fmt.Sprint("v", i))
					_ = vs[i].s.subscribe(map[string]int64{"log": int64(c.Version())})
				}
				b.ReportAllocs()
				n := 0
				for b.Loop() {
					n++
					_ = c.Upsert(row{ID: fmt.Sprint("n", n), Text: "an eighty byte row of log text, roughly what 17-tanstack measured per row"})
				}
				var total int64
				for _, v := range vs {
					v.stop()
					total += v.bytes.Load()
				}
				b.ReportMetric(float64(total)/float64(n)/float64(viewers), "B/frame")
			})
		}
	}
}

// BenchmarkCollectionMemory reports the heap a collection costs per item and a subscribed viewer costs on top.
func BenchmarkCollectionMemory(b *testing.B) {
	const items, viewers = 10000, 1000
	for b.Loop() {
		base := heapInUse()
		w := NewWorld()
		c := filled(w, "log", items)
		withItems := heapInUse()
		vs := make([]*viewer, viewers)
		for i := range vs {
			vs[i] = attachViewer(w, fmt.Sprint("v", i))
			_ = vs[i].s.subscribe(map[string]int64{"log": int64(c.Version())})
		}
		withViewers := heapInUse()
		b.ReportMetric(float64(withItems-base)/items, "B/item")
		b.ReportMetric(float64(withViewers-withItems)/viewers, "B/viewer")
		for _, v := range vs {
			v.stop()
		}
		runtime.KeepAlive(c)
	}
}

// BenchmarkCollectionReconnectStorm is every viewer subscribing at once from a version the history no longer holds:
// one reset is built and shared, so the cost per viewer stays flat as the set grows.
func BenchmarkCollectionReconnectStorm(b *testing.B) {
	const viewers = 1000
	for _, items := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("viewers=%d/items=%d", viewers, items), func(b *testing.B) {
			w := NewWorld()
			c := filled(w, "log", items).History(1)
			vs := make([]*viewer, viewers)
			for i := range vs {
				vs[i] = attachViewer(w, fmt.Sprint("v", i))
			}
			b.ReportAllocs()
			for b.Loop() {
				_ = c.Upsert(row{ID: "bump"}) // a new version, so the cached reset is rebuilt once per round
				var wg sync.WaitGroup
				for _, v := range vs {
					wg.Go(func() { _ = v.s.subscribe(map[string]int64{"log": 0}) })
				}
				wg.Wait()
			}
			for _, v := range vs {
				v.stop()
			}
		})
	}
}

// TestCollectionSessionsLeaveNoTrace runs pages through subscribe, drop, resume and expiry on synctest's clock: the
// sessions, subscriptions and goroutines all return to none.
func TestCollectionSessionsLeaveNoTrace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := NewWorld(WithResumeGrace(30 * time.Second))
		c := filled(w, "log", 100)
		baseline := runtime.NumGoroutine()
		for round := range 50 {
			vs := make([]*viewer, 20)
			for i := range vs {
				vs[i] = attachViewer(w, fmt.Sprintf("r%d-%d", round, i))
				_ = vs[i].s.subscribe(map[string]int64{"log": int64(c.Version()) - 3})
			}
			_ = c.Upsert(row{ID: fmt.Sprint("round", round)})
			for i, v := range vs {
				v.s.detach(v.c)
				if i%2 == 0 {
					// half resume within the grace, then leave for good
					_, cancel := context.WithCancel(context.Background())
					next := newConn(cancel)
					v.s.attach(next)
					v.s.detach(next)
				}
				v.stop()
			}
		}
		time.Sleep(31 * time.Second)
		synctest.Wait()
		w.mu.RLock()
		sessions := len(w.sessions)
		w.mu.RUnlock()
		c.mu.Lock()
		subs := len(c.subs)
		c.mu.Unlock()
		if sessions != 0 || subs != 0 {
			t.Fatalf("after expiry: %d sessions, %d subscribers left", sessions, subs)
		}
		if n := runtime.NumGoroutine(); n > baseline {
			t.Fatalf("goroutines: %d, baseline %d", n, baseline)
		}
	})
}
