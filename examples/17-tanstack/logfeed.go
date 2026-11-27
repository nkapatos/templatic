package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/tanstack/components"
)

// logFeed is the world state the log page shows.
type logFeed struct {
	rows *live.Collection[components.Row]
	rate *live.Cell[int]
	next int
}

// newLogFeed declares the log collection and the rate cell on w, fills the
// log with LogSize rows, and registers SetRate.
func newLogFeed(w *live.World) *logFeed {
	f := &logFeed{
		// No Tail: a new row goes on top; past LogSize the oldest drops, on
		// the server and on every page.
		rows: live.NewCollection(w, components.LogTopic, components.Row.Key).Limit(components.LogSize),
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
	return components.LogProps{Rows: f.rows.Snapshot()}
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

// add puts n made-up rows on top of the log, one op each. Only run's
// goroutine and newLogFeed call it, so next needs no lock.
func (f *logFeed) add(n int) {
	fresh := make([]components.Row, n)
	now := time.Now()
	for i := range fresh {
		f.next++
		fresh[i] = components.Row{
			ID:    f.next,
			At:    now.Format("15:04:05.000"),
			Level: levels[rand.IntN(len(levels))],
			Text:  fmt.Sprintf("%s %s %d in %dms", verbs[rand.IntN(len(verbs))], paths[rand.IntN(len(paths))], 200+rand.IntN(4)*100, 1+rand.IntN(400)),
		}
	}
	if err := f.rows.Upsert(fresh...); err != nil {
		log.Printf("tanstack: log: %v", err)
	}
}
