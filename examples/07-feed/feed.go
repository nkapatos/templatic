package main

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"

	"examples/feed/components"
)

// keepMessages and keepEvents bound each room and the activity log, on the
// server and on every open page.
const (
	keepMessages = 20
	keepEvents   = 10
)

// feed is the world's chat rooms and activity log; the collections hold the
// items, so there is no store beside them.
type feed struct {
	rooms    *live.CollectionFamily[components.Message]
	activity *live.Collection[components.Event]
	next     atomic.Int64
	open     atomic.Int64
}

// newFeed declares a collection per room and the activity log on w,
// registers the commands' JS handlers and logs session connects, resumes and
// ends.
func newFeed(w *live.World) *feed {
	f := &feed{
		// Tail: a room reads top to bottom, new messages at the bottom.
		rooms: live.NewCollectionFamily(w, "chat", components.Message.Key).
			Renders(components.MessageView).Elements("ol", "li").Tail().Limit(keepMessages),
		// No Tail: the newest event goes on top. The class comes from the item.
		activity: live.NewCollection(w, components.ActivityTopic, components.Event.Key).
			Renders(components.EventView).Elements("ul", "li").Limit(keepEvents).
			ItemAttrs(func(e components.Event) templ.Attributes { return templ.Attributes{"class": e.Kind} }),
	}
	w.OnConnect(func(*live.Session) { f.log("join", fmt.Sprintf("a page connected (%d open)", f.open.Add(1))) })
	w.OnResume(func(*live.Session) { f.log("resume", "a page reconnected") })
	w.OnDisconnect(func(*live.Session) { f.log("leave", fmt.Sprintf("a page left (%d open)", f.open.Add(-1))) })
	return f
}

// room returns a room's collection; rooms are validated before this, so the
// name is in the topic grammar.
func (f *feed) room(name string) *live.Collection[components.Message] {
	c, err := f.rooms.Get(name)
	if err != nil {
		panic(err)
	}
	return c
}

// post adds the message to its room on every page that shows the room.
func (f *feed) post(in components.PostIn) error {
	m := components.Message{
		ID:     int(f.next.Add(1)),
		Room:   in.Room,
		Author: strings.TrimSpace(in.Name),
		Text:   strings.TrimSpace(in.Text),
		At:     time.Now(),
	}
	if err := f.room(m.Room).Upsert(m); err != nil {
		return err
	}
	f.log("post", m.Author+" posted in #"+m.Room)
	return nil
}

// like counts one more like; the message is re-rendered in place on every page.
func (f *feed) like(in components.MessageIn) error {
	held, err := f.room(in.Room).Update(in.ID, func(m components.Message) components.Message {
		m.Likes++
		return m
	})
	if err == nil && !held {
		return live.NotFound("no such message")
	}
	return err
}

// remove deletes the message from every page.
func (f *feed) remove(in components.MessageIn) error {
	room := f.room(in.Room)
	if _, ok := room.Get(in.ID); !ok {
		return live.NotFound("no such message")
	}
	return room.Remove(in.ID)
}

// log puts an activity event on top of the log on every page.
func (f *feed) log(kind, text string) {
	e := components.Event{ID: int(f.next.Add(1)), Kind: kind, Text: text, At: time.Now()}
	if err := f.activity.Upsert(e); err != nil {
		log.Printf("feed: activity: %v", err)
	}
}

// tick logs the server's clock every d, with no request behind it.
func (f *feed) tick(d time.Duration) {
	for now := range time.Tick(d) {
		f.log("tick", "server clock "+now.Format("15:04:05"))
	}
}
