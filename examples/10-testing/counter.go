package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/testing/components"
)

// keepEntries bounds the log, on the server and on every open page.
const keepEntries = 10

// counter holds the shared count, the log and each open page's own adds.
type counter struct {
	count *live.Cell[int]
	log   *live.Collection[components.Entry]
	next  atomic.Int64

	mu    sync.Mutex
	pages map[string]components.PageAdds // by session id
}

// newCounter declares the count and the log on w and forgets a page's adds
// when its session ends.
func newCounter(w *live.World) *counter {
	c := &counter{
		count: live.NewCell(w, components.CountTopic, 0),
		// Tail: the log reads top to bottom, the newest entry at the bottom.
		log: live.NewCollection(w, components.LogTopic, components.Entry.Key).
			Renders(components.EntryView).Elements("ol", "li").Tail().Limit(keepEntries),
		pages: map[string]components.PageAdds{},
	}
	w.OnDisconnect(func(s *live.Session) {
		c.mu.Lock()
		delete(c.pages, s.SID())
		c.mu.Unlock()
	})
	return c
}

// addForm serves Add at the form's action URL; a page with a live session
// also gets its own tally swapped. Without JavaScript the browser is sent
// back to the page, and refused input goes to invalid.
func (c *counter) addForm(w *live.World, invalid func(http.ResponseWriter, *http.Request, error)) http.Handler {
	return components.Add.Form(w, func(_ *http.Request, s *live.Session, in components.AddIn) (string, error) {
		if err := c.add(in.N); err != nil {
			return "", err
		}
		if s != nil {
			if err := components.TallyRegion.Swap(s, c.mark(s.SID(), in.N)); err != nil {
				return "", err
			}
		}
		return "/", nil
	}, invalid)
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

// append puts an entry at the bottom of the log on every page.
func (c *counter) append(kind, text string) error {
	e := components.Entry{ID: int(c.next.Add(1)), Kind: kind, Text: text, At: time.Now()}
	return c.log.Upsert(e)
}
