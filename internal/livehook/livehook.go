// Package livehook bridges live to livetest without widening live's public API.
// live fills in the function variables from its test hook file; livetest calls them.
package livehook

import (
	"encoding/json/jsontext"
	"net/http"
	"net/url"
)

// Event is one recorded wire event.
type Event struct {
	Kind  string
	Topic string
	Body  jsontext.Value
}

// Conn is an in-memory connection attached to a session.
type Conn struct {
	Session any
	// Drain returns the events offered since the last call.
	Drain func() []Event
	// Close detaches and retires the session.
	Close func()
	// Subscribe subscribes the session to collections, topic → the version the page holds (-1 for none).
	Subscribe func(held map[string]int64) error
	// Drop detaches the connection; the session stays resumable for the world's grace.
	Drop func()
	// Resume attaches a new connection to a dropped session; false means it is gone and the page would resync.
	Resume func() bool
}

var (
	// Connect registers session sid on world as if its stream connected with r
	// and a page token carrying params.
	Connect func(world any, sid string, params map[string]string, r *http.Request) Conn
	// Dispatch runs action on the session, with form args when form is non-nil.
	Dispatch func(session any, action string, args jsontext.Value, form url.Values, r *http.Request) (any, error)
	// CellValue returns the current {"v":…} body of a session or world cell topic.
	CellValue func(session any, topic string) (jsontext.Value, bool)
)
