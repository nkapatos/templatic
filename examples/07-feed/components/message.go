package components

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nkapatos/templatic/live"
)

// Rooms are the chat rooms, in display order; a room outside it is a 404.
var Rooms = []string{"general", "random"}

// Message is one chat line.
type Message struct {
	ID     int
	Room   string
	Author string
	Text   string
	At     time.Time
	Likes  int
}

// Key is the message's item key, unique across rooms.
func (m Message) Key() string { return "m" + strconv.Itoa(m.ID) }

// PostIn is Post's input, from either transport.
type PostIn struct {
	// Room is the {room} route wildcard.
	Room string `json:"room"`
	Name string `json:"name"`
	Text string `json:"text"`
}

// Validate refuses an unknown room, an empty or long name and an empty or
// long message.
func (in PostIn) Validate() error {
	var errs []error
	if !slices.Contains(Rooms, in.Room) {
		errs = append(errs, live.Invalid("", "unknown room"))
	}
	switch n := strings.TrimSpace(in.Name); {
	case n == "":
		errs = append(errs, live.Invalid("name", "enter a name"))
	case utf8.RuneCountInString(n) > 20:
		errs = append(errs, live.Invalid("name", "20 characters at most"))
	}
	switch t := strings.TrimSpace(in.Text); {
	case t == "":
		errs = append(errs, live.Invalid("text", "write something"))
	case utf8.RuneCountInString(t) > 280:
		errs = append(errs, live.Invalid("text", "280 characters at most"))
	}
	return errors.Join(errs...)
}

// Post adds a message to a room.
var Post = live.NewCommand[PostIn]("chat.post")

// MessageIn names one message, for Like and Delete.
type MessageIn struct {
	Room string `json:"room"`
	ID   string `json:"id"`
}

// Validate refuses an unknown room or an empty id.
func (in MessageIn) Validate() error {
	if !slices.Contains(Rooms, in.Room) || in.ID == "" {
		return live.Invalid("", "no such message")
	}
	return nil
}

// Like adds a like to a message: an update of an item already on the page.
var Like = live.NewCommand[MessageIn]("chat.like")

// Delete removes a message from its room on every page.
var Delete = live.NewCommand[MessageIn]("chat.delete")

// Event is one line of the activity log.
type Event struct {
	ID   int
	Kind string // "post", "tick", "join", "resume" or "leave": the item's class
	Text string
	At   time.Time
}

// Key is the event's item key.
func (e Event) Key() string { return "e" + strconv.Itoa(e.ID) }

// ActivityTopic is the activity log's collection topic.
const ActivityTopic = "activity"

func messageURL(m Message) string { return "/rooms/" + m.Room + "/messages/" + m.Key() }
