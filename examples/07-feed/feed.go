package main

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"

	"examples/feed/components"
)

// keepMessages and keepEvents bound what the server keeps and what a page
// gets on load or reconnect.
const (
	keepMessages = 20
	keepEvents   = 10
)

// feed holds the last messages of each room and the last activity events.
type feed struct {
	w      *live.World
	mu     sync.Mutex
	next   int
	rooms  map[string][]components.Message
	events []components.Event
	open   atomic.Int64
}

// newFeed binds a stream per room and the activity stream on w, registers
// Post's JS handler and logs session connects, resumes and ends.
func newFeed(w *live.World) *feed {
	f := &feed{w: w, rooms: map[string][]components.Message{}}
	for _, room := range components.Rooms {
		// Bind gives the stream a replay: a reconnecting page gets the
		// room's kept messages swapped into the region around the stream.
		components.Chat.For(room).Bind(w, func() []components.Message { return f.messages(room) })
	}
	w.Stream(components.ActivityTopic, func() templ.Component { return components.Activity(f.recent()) })

	components.Post.Handle(w, func(_ *live.Session, in components.PostIn) (any, error) {
		return nil, f.post(in)
	})
	w.OnConnect(func(*live.Session) { f.log("join", fmt.Sprintf("a page connected (%d open)", f.open.Add(1))) })
	w.OnResume(func(*live.Session) { f.log("resume", "a page reconnected") })
	w.OnDisconnect(func(*live.Session) { f.log("leave", fmt.Sprintf("a page left (%d open)", f.open.Add(-1))) })
	return f
}

// post keeps the message and appends it to its room's stream on every page.
func (f *feed) post(in components.PostIn) error {
	f.mu.Lock()
	f.next++
	m := components.Message{
		ID:     f.next,
		Room:   in.Room,
		Author: strings.TrimSpace(in.Name),
		Text:   strings.TrimSpace(in.Text),
		At:     time.Now(),
	}
	f.rooms[m.Room] = keepLast(append(f.rooms[m.Room], m), keepMessages)
	// Appending under the lock keeps the pages' order the store's order.
	err := components.Chat.For(m.Room).Append(f.w, m)
	f.mu.Unlock()
	if err != nil {
		return err
	}
	f.log("post", m.Author+" posted in #"+m.Room)
	return nil
}

// log keeps an activity event and prepends it on every page.
func (f *feed) log(kind, text string) {
	f.mu.Lock()
	f.next++
	e := components.Event{ID: f.next, Kind: kind, Text: text, At: time.Now()}
	f.events = keepLast(append(f.events, e), keepEvents)
	err := f.w.Append(components.ActivityTopic, e.Key(), components.EventView(e), live.ItemClass(e.Kind))
	f.mu.Unlock()
	if err != nil {
		log.Printf("feed: append: %v", err)
	}
}

// tick logs the server's clock every d, with no request behind it.
func (f *feed) tick(d time.Duration) {
	for now := range time.Tick(d) {
		f.log("tick", "server clock "+now.Format("15:04:05"))
	}
}

// messages returns a room's kept messages, oldest first.
func (f *feed) messages(room string) []components.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.rooms[room])
}

// recent returns the kept events, newest first: the activity stream
// prepends, so its first paint is in that order too.
func (f *feed) recent() []components.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.events)
	slices.Reverse(out)
	return out
}

func keepLast[T any](s []T, n int) []T {
	if len(s) > n {
		return slices.Clone(s[len(s)-n:])
	}
	return s
}
