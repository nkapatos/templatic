package main

import (
	"log"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"

	"examples/clientlibs/components"
)

var errNoTask = live.Invalid("id", "no such task")

// board is the shared queue and stats, each a world region.
type board struct {
	w     *live.World
	queue *live.Region
	stats *live.Region

	mu    sync.Mutex
	tasks []components.Task
	s     components.Stats
}

// newBoard declares the queue and stats regions on w.
func newBoard(w *live.World) *board {
	now := time.Now()
	b := &board{w: w, s: components.Stats{Revenue: 1234.5, Currency: "EUR", Visitors: 9876, Reordered: now, Now: now}}
	for _, t := range [][2]string{{"docs", "Write the docs"}, {"tests", "Fix flaky tests"}, {"tag", "Tag v0.1.0"}, {"notes", "Publish release notes"}, {"party", "Celebrate"}} {
		b.tasks = append(b.tasks, components.Task{ID: t[0], Title: t[1]})
	}
	b.queue = w.Region("queue", func() templ.Component { return components.Queue(b.allTasks()) })
	b.stats = w.Region("stats", func() templ.Component { return components.StatsView(b.allStats()) })
	return b
}

func (b *board) allTasks() []components.Task {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.tasks)
}

func (b *board) allStats() components.Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.s
}

// move is Move's step for both transports. To is clamped to the queue, so
// a stale index from a page that missed a redraw still lands somewhere valid.
func (b *board) move(in components.MoveIn) error {
	b.mu.Lock()
	i := slices.IndexFunc(b.tasks, func(t components.Task) bool { return t.ID == in.ID })
	if i < 0 {
		b.mu.Unlock()
		return errNoTask
	}
	t := b.tasks[i]
	b.tasks = slices.Insert(slices.Delete(b.tasks, i, i+1), min(max(in.To, 0), len(b.tasks)-1), t)
	b.s.Reordered = time.Now()
	b.mu.Unlock()
	b.redraw(b.queue, b.stats)
	return nil
}

// tick changes the stats every interval; it never returns.
func (b *board) tick(interval time.Duration) {
	for range time.Tick(interval) {
		b.mu.Lock()
		b.s.Revenue += float64(rand.IntN(5000)) / 100
		b.s.Visitors += rand.IntN(20)
		b.s.Now = time.Now()
		b.mu.Unlock()
		b.redraw(b.stats)
	}
}

func (b *board) redraw(rs ...*live.Region) {
	for _, r := range rs {
		if err := b.w.Redraw(r.Name()); err != nil {
			log.Printf("clientlibs: redraw %s: %v", r.Name(), err)
		}
	}
}
