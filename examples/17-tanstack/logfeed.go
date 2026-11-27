package main

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/tanstack/components"
)

// logFeed is the world state the log page shows.
type logFeed struct {
	rows *live.Cell[[]components.Row]
	rate *live.Cell[int]
	next int
}

// newLogFeed declares the log cells on w, filled with LogSize rows, and
// registers SetRate.
func newLogFeed(w *live.World) *logFeed {
	f := &logFeed{
		rows: live.NewCell(w, "log.rows", []components.Row{}),
		rate: live.NewCell(w, "log.rate", 1),
	}
	f.add(components.LogSize)
	components.SetRate.Handle(w, func(_ *live.Session, in components.RateIn) (any, error) {
		f.rate.Set(in.PerSecond)
		return nil, nil
	})
	return f
}

// props is the island's props at render time.
func (f *logFeed) props() components.LogProps {
	return components.LogProps{Rows: f.rows.Get()}
}

// run adds one row at a time at the rate the rate cell says; it never returns.
func (f *logFeed) run() {
	for {
		if n := f.rate.Get(); n > 0 {
			time.Sleep(time.Second / time.Duration(n))
			f.add(1)
		} else {
			time.Sleep(200 * time.Millisecond)
		}
	}
}

var (
	levels = []string{"info", "info", "info", "warn", "error"}
	verbs  = []string{"GET", "POST", "PUT", "DELETE"}
	paths  = []string{"/orders", "/orders/{id}", "/users", "/search", "/live/stream", "/signup"}
)

// add puts n made-up rows on top of the log and drops the oldest past
// LogSize. Only run's goroutine and newLogFeed call it, so next needs no lock.
func (f *logFeed) add(n int) {
	fresh := make([]components.Row, n)
	now := time.Now()
	for i := range fresh {
		f.next++
		fresh[n-1-i] = components.Row{
			ID:    f.next,
			At:    now.Format("15:04:05.000"),
			Level: levels[rand.IntN(len(levels))],
			Text:  fmt.Sprintf("%s %s %d in %dms", verbs[rand.IntN(len(verbs))], paths[rand.IntN(len(paths))], 200+rand.IntN(4)*100, 1+rand.IntN(400)),
		}
	}
	f.rows.Update(func(old []components.Row) []components.Row {
		// A new slice: the old one may still be read by a render.
		return slices.Concat(fresh, old[:min(len(old), components.LogSize-n)])
	})
}
