// Package livetest runs live commands against an in-memory session in tests.
//
// [New] connects a [Session] to a [live.World] without HTTP or a browser. The
// session records every event the world sends it, and the helpers run
// registered commands through their handlers and input decoding, skipping
// the HTTP layer: [Call] and [Session.Do] with JSON-shaped input, [Submit] and
// [Session.DoForm] with form fields. The test then reads what was pushed:
// [Session.Swaps], [Session.LastSwap], [Session.Appends], [Session.Cell] and
// [Session.Events]. [Session.WaitSwap] polls for a swap that arrives
// asynchronously and fails the test on timeout. The session is retired when
// the test ends.
//
// A run returns the command handler's result and error as is; a refused input
// comes back as an error that [live.InputErrors] can list.
//
//	func TestAdd(t *testing.T) {
//		w := live.NewWorld()
//		count := live.NewCell(w, "count", 0)
//		Add.Handle(w, func(s *live.Session, in AddIn) (any, error) {
//			count.Update(func(v int) int { return v + in.N })
//			return nil, nil
//		})
//
//		s := livetest.New(t, w)
//		if _, err := livetest.Call(s, Add, AddIn{N: 2}); err != nil {
//			t.Fatal(err)
//		}
//		var got int
//		if !s.Cell("count", &got) || got != 2 {
//			t.Fatalf("count = %d", got)
//		}
//	}
//
// This package is for tests only.
package livetest

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nkapatos/templatic/internal/livehook"
	"github.com/nkapatos/templatic/live"
)

// Event is one recorded wire event; Body is the raw payload.
type Event = livehook.Event

// Session is a connected in-memory session; its embedded *live.Session is what
// handlers receive.
type Session struct {
	*live.Session
	t    testing.TB
	conn livehook.Conn

	mu     sync.Mutex
	events []Event
}

// Option configures New.
type Option func(*config)

type config struct {
	sid string
	req *http.Request
}

// SID sets the session id (default: a unique one per New).
func SID(sid string) Option { return func(c *config) { c.sid = sid } }

// Request sets the request the connect and every Call are captured from.
func Request(r *http.Request) Option { return func(c *config) { c.req = r } }

var seq atomic.Uint64

// New connects a session to w; it is retired when the test ends.
func New(t testing.TB, w *live.World, opts ...Option) *Session {
	t.Helper()
	cfg := config{sid: fmt.Sprintf("livetest-%d", seq.Add(1))}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.req == nil {
		cfg.req = httptest.NewRequest("GET", "/", nil)
	}
	conn := livehook.Connect(w, cfg.sid, cfg.req)
	s := &Session{Session: conn.Session.(*live.Session), t: t, conn: conn}
	t.Cleanup(conn.Close)
	return s
}

// Do runs action with args (marshaled to JSON; nil for none).
func (s *Session) Do(action string, args any) (any, error) {
	s.t.Helper()
	var raw jsontext.Value
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			s.t.Fatalf("livetest: args for %s: %v", action, err)
		}
		raw = b
	}
	res, err := livehook.Dispatch(s.Session, action, raw, nil, httptest.NewRequest("POST", "/live/commands", nil))
	s.collect()
	return res, err
}

// Call runs a declared command with a typed input.
func Call[In any](s *Session, cmd *live.Command[In], in In) (any, error) {
	s.t.Helper()
	return s.Do(cmd.Name(), in)
}

// DoForm runs action as a form submit with these fields.
func (s *Session) DoForm(action string, form url.Values) (any, error) {
	s.t.Helper()
	if form == nil {
		form = url.Values{}
	}
	res, err := livehook.Dispatch(s.Session, action, nil, form, httptest.NewRequest("POST", "/live/commands", nil))
	s.collect()
	return res, err
}

// Submit runs a declared command as a form submit with these fields.
func Submit[In any](s *Session, cmd *live.Command[In], form url.Values) (any, error) {
	s.t.Helper()
	return s.DoForm(cmd.Name(), form)
}

func (s *Session) collect() []Event {
	evs := s.conn.Drain()
	s.mu.Lock()
	s.events = append(s.events, evs...)
	out := append([]Event(nil), s.events...)
	s.mu.Unlock()
	return out
}

// Events returns every event recorded since New (or Reset).
func (s *Session) Events() []Event { return s.collect() }

// Reset forgets the recorded events.
func (s *Session) Reset() {
	s.collect()
	s.mu.Lock()
	s.events = nil
	s.mu.Unlock()
}

// Swaps returns the HTML of every swap of topic, oldest first.
func (s *Session) Swaps(topic string) []string {
	var out []string
	for _, ev := range s.collect() {
		if ev.Kind == live.KindSwap && ev.Topic == topic {
			var b struct {
				HTML string `json:"html"`
			}
			_ = json.Unmarshal(ev.Body, &b)
			out = append(out, b.HTML)
		}
	}
	return out
}

// LastSwap returns the latest swap of topic.
func (s *Session) LastSwap(topic string) (string, bool) {
	sw := s.Swaps(topic)
	if len(sw) == 0 {
		return "", false
	}
	return sw[len(sw)-1], true
}

// Appends returns the item HTML of every append to stream topic.
func (s *Session) Appends(topic string) []string {
	var out []string
	for _, ev := range s.collect() {
		if ev.Kind == live.KindAppend && ev.Topic == topic {
			var b struct {
				Items []struct {
					HTML string `json:"html"`
				} `json:"items"`
			}
			_ = json.Unmarshal(ev.Body, &b)
			for _, it := range b.Items {
				out = append(out, it.HTML)
			}
		}
	}
	return out
}

// Cell decodes the current value of cell topic into v and reports whether it
// exists.
func (s *Session) Cell(topic string, v any) bool {
	s.t.Helper()
	body, ok := livehook.CellValue(s.Session, topic)
	if !ok {
		return false
	}
	var env struct {
		V jsontext.Value `json:"v"`
	}
	if err := json.Unmarshal(body, &env); err != nil || json.Unmarshal(env.V, v) != nil {
		s.t.Fatalf("livetest: cell %s = %s does not decode into %T", topic, body, v)
	}
	return true
}

// WaitSwap waits up to d for a swap of topic containing substr.
func (s *Session) WaitSwap(topic, substr string, d time.Duration) string {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for {
		for _, h := range s.Swaps(topic) {
			if strings.Contains(h, substr) {
				return h
			}
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("livetest: no swap of %s containing %q within %v", topic, substr, d)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
