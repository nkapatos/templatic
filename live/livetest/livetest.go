// Package livetest runs live commands against an in-memory session in tests.
//
// [New] connects a [Session] to a [live.World] without HTTP or a browser. The
// session records every event the world sends it, and the helpers run
// registered commands through their handlers and input decoding, skipping
// the HTTP layer: [Call] and [Session.Do] with JSON-shaped input, [Submit] and
// [Session.DoForm] with form fields. The test then reads what was pushed:
// [Session.Swaps], [Session.LastSwap], [Session.Ops], [Session.Cell] and
// [Session.Events]. [Session.WaitSwap] and [Session.WaitOp] poll for a swap or
// a collection change that arrives asynchronously and fail the test on
// timeout. A session receives a collection's changes once it shows it
// ([Session.Show]), as a page that rendered it does. [Session.Drop] and
// [Session.Resume] simulate a dropped and resumed connection. The session is
// retired when the test ends.
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
	"slices"
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
	req  *http.Request // New's request; commands carry its header

	mu     sync.Mutex
	events []Event
	shown  map[string]uint64 // collections shown, at the version Show gave
}

// Option configures New.
type Option func(*config)

type config struct {
	sid    string
	req    *http.Request
	params map[string]string
}

// SID sets the session id (default: a unique one per New).
func SID(sid string) Option { return func(c *config) { c.sid = sid } }

// Request sets the request the connect is captured from; every command run
// carries its header, cookies included.
func Request(r *http.Request) Option { return func(c *config) { c.req = r } }

// Param adds a page-token param, as [live.TokenParam] would; handlers read it
// from [live.Session.Params].
func Param(key, value string) Option {
	return func(c *config) {
		if c.params == nil {
			c.params = map[string]string{}
		}
		c.params[key] = value
	}
}

var seq atomic.Uint64

// New connects a session to w, running w's OnConnect hooks; it is retired when the test ends.
func New(t testing.TB, w *live.World, opts ...Option) *Session {
	t.Helper()
	cfg := config{sid: fmt.Sprintf("livetest-%d", seq.Add(1))}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.req == nil {
		cfg.req = httptest.NewRequest("GET", "/", nil)
	}
	conn := livehook.Connect(w, cfg.sid, cfg.params, cfg.req)
	s := &Session{Session: conn.Session.(*live.Session), t: t, conn: conn, req: cfg.req}
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
	res, err := livehook.Dispatch(s.Session, action, raw, nil, s.commandRequest())
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
	res, err := livehook.Dispatch(s.Session, action, nil, form, s.commandRequest())
	s.collect()
	return res, err
}

// Submit runs a declared command as a form submit with these fields.
func Submit[In any](s *Session, cmd *live.Command[In], form url.Values) (any, error) {
	s.t.Helper()
	return s.DoForm(cmd.Name(), form)
}

// PostForm sends h the POST the runtime makes when the page submits a form to target with these fields, and
// returns the answer: 204, or the error's status (400 carries the input errors).
func (s *Session) PostForm(h http.Handler, target string, fields url.Values) *httptest.ResponseRecorder {
	s.t.Helper()
	r := httptest.NewRequest("POST", target, strings.NewReader(fields.Encode()))
	for k, v := range s.req.Header {
		r.Header[k] = slices.Clone(v)
	}
	r.RemoteAddr = s.req.RemoteAddr
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set(live.FormHeader, "1")
	r.Header.Set(live.SidHeader, s.SID())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	s.collect()
	return rec
}

func (s *Session) commandRequest() *http.Request {
	r := httptest.NewRequest("POST", "/live/commands", nil)
	r.Header = s.req.Header.Clone()
	r.RemoteAddr = s.req.RemoteAddr
	return r
}

func (s *Session) collect() []Event {
	evs := s.conn.Drain()
	s.mu.Lock()
	for _, ev := range evs {
		// a shown collection's page now holds the version this event ends at
		if held, ok := s.shown[ev.Topic]; ok && (ev.Kind == live.KindOps || ev.Kind == live.KindReset) {
			var b struct {
				Ver uint64 `json:"ver"`
			}
			if json.Unmarshal(ev.Body, &b) == nil && b.Ver > held {
				s.shown[ev.Topic] = b.Ver
			}
		}
	}
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

// Op is one recorded collection change at Version: Kind "u" (an upsert), "d" (a removal) or "reset" (every item,
// Key empty). A server-rendered collection's change carries HTML; a data collection's carries Value, the item's
// JSON, or for a reset the items as [{"k","v"}…].
type Op struct {
	Kind    string
	Key     string
	HTML    string
	Value   jsontext.Value
	Version uint64
}

// Show subscribes the session to collection topic as a page that rendered it at version held does; it then
// receives what that page lacks, and every later change.
func (s *Session) Show(topic string, held uint64) {
	s.t.Helper()
	if err := s.conn.Subscribe(map[string]int64{topic: int64(held)}); err != nil {
		s.t.Fatalf("livetest: show %s: %v", topic, err)
	}
	s.mu.Lock()
	if s.shown == nil {
		s.shown = map[string]uint64{}
	}
	s.shown[topic] = max(s.shown[topic], held)
	s.mu.Unlock()
	s.collect()
}

// Ops returns every recorded change of collection topic, oldest first.
func (s *Session) Ops(topic string) []Op {
	var out []Op
	for _, ev := range s.collect() {
		if ev.Topic != topic {
			continue
		}
		var b struct {
			Ver   uint64         `json:"ver"`
			HTML  string         `json:"html"`
			Items jsontext.Value `json:"items"`
			Ops   []struct {
				Op   string         `json:"op"`
				K    string         `json:"k"`
				HTML string         `json:"html"`
				V    jsontext.Value `json:"v"`
			} `json:"ops"`
		}
		switch ev.Kind {
		case live.KindOps:
			_ = json.Unmarshal(ev.Body, &b)
			first := b.Ver - uint64(len(b.Ops)) + 1
			for i, op := range b.Ops {
				out = append(out, Op{Kind: op.Op, Key: op.K, HTML: op.HTML, Value: op.V, Version: first + uint64(i)})
			}
		case live.KindReset:
			_ = json.Unmarshal(ev.Body, &b)
			out = append(out, Op{Kind: "reset", HTML: b.HTML, Value: b.Items, Version: b.Ver})
		}
	}
	return out
}

// WaitOp waits up to d for a change of collection topic whose HTML or Value contains substr (any change when substr
// is "").
func (s *Session) WaitOp(topic, substr string, d time.Duration) Op {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for {
		for _, op := range s.Ops(topic) {
			if strings.Contains(op.HTML, substr) || strings.Contains(string(op.Value), substr) {
				return op
			}
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("livetest: no change of %s containing %q within %v", topic, substr, d)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Drop detaches the session's connection, as a page losing its stream does; the session stays resumable for the
// world's grace, and changes published meanwhile are not recorded.
func (s *Session) Drop() {
	s.collect()
	s.conn.Drop()
}

// Resume reattaches a dropped session and shows its collections again at the versions it last received, as the
// page would on reconnect; false means the session is gone and the page would resync.
func (s *Session) Resume() bool {
	s.t.Helper()
	if !s.conn.Resume() {
		return false
	}
	held := map[string]int64{}
	s.mu.Lock()
	for topic, v := range s.shown {
		held[topic] = int64(v)
	}
	s.mu.Unlock()
	if err := s.conn.Subscribe(held); err != nil {
		s.t.Fatalf("livetest: resume: %v", err)
	}
	s.collect()
	return true
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
