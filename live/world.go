package live

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a-h/templ"
)

// World is the reactive surface a set of pages shares: cells, regions, commands and
// sessions.
type World struct {
	mu           sync.RWMutex
	cells        map[string]cellView
	regions      map[string]*Region
	collections  map[string]subscribable
	commands     map[string]CommandHandler
	sessions     map[string]*Session
	onConnect    []func(*Session)
	onDisconnect []func(*Session)
	onResume     []func(*Session)
	grace        time.Duration
	key          []byte
	devKey       []byte
	devKeyOnce   sync.Once
	tokenTTL     time.Duration
	now          func() time.Time
	reqHeaders   []string
	reqCookies   []string
	heartbeat    time.Duration
	writeTimeout time.Duration
	coalesce     time.Duration
	submits      submitStore
	swapGen      atomic.Uint64 // stamps swaps in the order their state was read
}

// Option configures a World at construction.
type Option func(*World)

// WithHeartbeat sets the SSE keepalive interval and per-frame write deadline.
func WithHeartbeat(interval, writeTimeout time.Duration) Option {
	return func(w *World) {
		if interval > 0 {
			w.heartbeat = interval
		}
		if writeTimeout > 0 {
			w.writeTimeout = writeTimeout
		}
	}
}

// WithCoalesceWindow sets how long a session gathers a burst of publishes into one
// SSE flush (default 8ms); a burst's first event waits this long. d <= 0 keeps the default.
func WithCoalesceWindow(d time.Duration) Option {
	return func(w *World) {
		if d > 0 {
			w.coalesce = d
		}
	}
}

// DefaultResumeGrace is how long a dropped session stays resumable.
const DefaultResumeGrace = 30 * time.Second

// WithResumeGrace sets how long a session survives the drop of its SSE connection
// (default 30s).
func WithResumeGrace(d time.Duration) Option {
	return func(w *World) { w.grace = d }
}

// NewWorld creates an empty world.
func NewWorld(opts ...Option) *World {
	w := &World{
		cells:        map[string]cellView{},
		regions:      map[string]*Region{},
		collections:  map[string]subscribable{},
		commands:     map[string]CommandHandler{},
		sessions:     map[string]*Session{},
		grace:        DefaultResumeGrace,
		tokenTTL:     DefaultTokenTTL,
		heartbeat:    15 * time.Second,
		writeTimeout: 10 * time.Second,
		coalesce:     coalesceWindow,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *World) resumeGrace() time.Duration { return w.grace }

func (w *World) heartbeatInterval() time.Duration { return w.heartbeat }

func (w *World) writeDeadline() time.Duration { return w.writeTimeout }

func (w *World) coalesceWindowOf() time.Duration { return w.coalesce }

// NewCell declares, or returns, the typed cell published under topic to every
// session of w.
func NewCell[T any](w *World, topic string, initial T) *Cell[T] {
	validateTopic(topic)
	w.mu.Lock()
	defer w.mu.Unlock()
	if existing, ok := w.cells[topic]; ok {
		if c, ok := existing.(*Cell[T]); ok {
			return c
		}
	}
	c := &Cell[T]{world: w, name: topic, value: initial}
	w.cells[topic] = c
	return c
}

// Region declares a redrawable region under topic.
func (w *World) Region(topic string, render func() templ.Component) *Region {
	validateTopic(topic)
	w.mu.Lock()
	defer w.mu.Unlock()
	if r, ok := w.regions[topic]; ok {
		r.render = render
		return r
	}
	r := &Region{world: w, name: topic, render: render}
	w.regions[topic] = r
	return r
}

// Redraw re-renders the named region and pushes the markup to all sessions.
func (w *World) Redraw(topic string) error {
	w.mu.RLock()
	r, ok := w.regions[topic]
	w.mu.RUnlock()
	if !ok {
		return &UnknownRegionError{Region: topic}
	}
	gen := w.swapGen.Add(1)
	html, err := r.redraw()
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		HTML string `json:"html"`
	}{HTML: html})
	if err != nil {
		return err
	}
	w.publish(event{kind: KindSwap, topic: topic, body: body, version: gen})
	return nil
}

// UnknownRegionError reports a Redraw call against an undeclared region.
type UnknownRegionError struct{ Region string }

func (e *UnknownRegionError) Error() string {
	return "live: unknown region " + e.Region
}

// Command registers an untyped handler for action.
func (w *World) Command(action string, fn CommandHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.commands[action] = fn
}

// OnConnect registers a callback for new sessions.
func (w *World) OnConnect(fn func(*Session)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onConnect = append(w.onConnect, fn)
}

// OnResume registers a callback for a session that reattached within the resume
// grace.
func (w *World) OnResume(fn func(*Session)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onResume = append(w.onResume, fn)
}

// OnDisconnect registers a callback that runs once when a session ends.
func (w *World) OnDisconnect(fn func(*Session)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onDisconnect = append(w.onDisconnect, fn)
}

func (w *World) publish(ev event) {
	w.mu.RLock()
	sessions := make([]*Session, 0, len(w.sessions))
	for _, s := range w.sessions {
		sessions = append(sessions, s)
	}
	w.mu.RUnlock()
	for _, s := range sessions {
		s.offer(ev)
	}
}

func (w *World) liveSessions() []*Session {
	w.mu.RLock()
	out := make([]*Session, 0, len(w.sessions))
	for _, s := range w.sessions {
		out = append(out, s)
	}
	w.mu.RUnlock()
	kept := out[:0]
	for _, s := range out {
		if !s.isRetired() {
			kept = append(kept, s)
		}
	}
	return kept
}

func (w *World) snapshot() []event {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]event, 0, len(w.cells))
	for name, c := range w.cells {
		if body, version := c.snapshot(); body != nil {
			out = append(out, event{kind: KindCell, topic: name, body: body, version: version})
		}
	}
	return out
}

// SSE returns the handler that sessions connect to over Server-Sent Events.
func (w *World) SSE() http.Handler { return &streamHandler{world: w} }

// Commands returns the POST handler that dispatches page actions into world command
// handlers.
func (w *World) Commands() http.Handler { return &commandsHandler{world: w} }

// Mount registers the world's handlers on mux under base.
func (w *World) Mount(mux *http.ServeMux, base string) {
	mux.Handle("GET "+trimSlash(base)+"/stream", w.SSE())
	mux.Handle("POST "+trimSlash(base)+"/commands", w.Commands())
}

func trimSlash(s string) string {
	if len(s) > 0 && s[len(s)-1] == '/' {
		return s[:len(s)-1]
	}
	return s
}

// Topics are spliced unescaped into SSE frames and selectors; "" panics too, since the coalescer treats it as kind-level.
var errTopic = errors.New("live: topic must match [A-Za-z0-9_.-]+")

func validateTopic(topic string) {
	if err := checkTopic(topic); err != nil {
		panic(err)
	}
}

// checkTopic returns an error instead of panicking, for topics built from data.
func checkTopic(topic string) error {
	if topic == "" {
		return errTopic
	}
	for _, r := range topic {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
		default:
			return errTopic
		}
	}
	return nil
}
