package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/testing/components"
)

// keepEntries bounds the log the server keeps and a page gets on load or reconnect.
const keepEntries = 10

// counter holds the shared count, the kept log and each open page's own adds.
type counter struct {
	w     *live.World
	count *live.Cell[int]

	mu    sync.Mutex
	next  int
	log   []components.Entry
	pages map[string]components.PageAdds // by session id
}

// newCounter declares the count and the log on w, registers Add's JS handler
// and forgets a page's adds when its session ends.
func newCounter(w *live.World) *counter {
	c := &counter{w: w, count: live.NewCell(w, components.CountTopic, 0), pages: map[string]components.PageAdds{}}
	components.Log.Bind(w, c.entries)
	components.Add.Handle(w, func(s *live.Session, in components.AddIn) (any, error) {
		if err := c.add(in.N); err != nil {
			return nil, err
		}
		return nil, components.TallyRegion.Swap(s, c.mark(s.SID(), in.N))
	})
	w.OnDisconnect(func(s *live.Session) {
		c.mu.Lock()
		delete(c.pages, s.SID())
		c.mu.Unlock()
	})
	return c
}

// add adds n to the count and logs it on every page.
func (c *counter) add(n int) error {
	var total int
	c.count.Update(func(v int) int { total = v + n; return total })
	return c.append("add", fmt.Sprintf("%+d → %d", n, total))
}

// mark records an add by the page of session sid and returns its new tally.
func (c *counter) mark(sid string, n int) components.PageAdds {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.pages[sid]
	a.Count++
	a.Last = n
	c.pages[sid] = a
	return a
}

// tick logs the server's clock every d until ctx ends.
func (c *counter) tick(ctx context.Context, d time.Duration) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if err := c.append("tick", "server clock "+now.Format("15:04:05")); err != nil {
				log.Printf("testing: tick: %v", err)
			}
		}
	}
}

func (c *counter) append(kind, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	e := components.Entry{ID: c.next, Kind: kind, Text: text, At: time.Now()}
	c.log = append(c.log, e)
	if len(c.log) > keepEntries {
		c.log = slices.Clone(c.log[len(c.log)-keepEntries:])
	}
	// Appending under the lock keeps the pages' order the log's order.
	return components.Log.Append(c.w, e)
}

// entries returns the kept log, oldest first.
func (c *counter) entries() []components.Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.log)
}
