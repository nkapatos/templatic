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
}

var (
	// Connect registers session sid on world as if its stream connected with r.
	Connect func(world any, sid string, r *http.Request) Conn
	// Dispatch runs action on the session, with form args when form is non-nil.
	Dispatch func(session any, action string, args jsontext.Value, form url.Values, r *http.Request) (any, error)
	// CellValue returns the current {"v":…} body of a session or world cell topic.
	CellValue func(session any, topic string) (jsontext.Value, bool)
)
