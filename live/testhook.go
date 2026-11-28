package live

import (
	"context"
	"encoding/json/jsontext"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sync"

	"github.com/nkapatos/templatic/internal/livehook"
)

func init() {
	livehook.Connect = func(wa any, sid string, params map[string]string, r *http.Request) livehook.Conn {
		w := wa.(*World)
		s := &Session{world: w, sid: sid, params: maps.Clone(params)}
		w.mu.Lock()
		w.sessions[sid] = s
		onConnect := slices.Clone(w.onConnect)
		w.mu.Unlock()
		info := w.captureRequest(r, s.params)
		s.mu.Lock()
		s.req = info
		s.mu.Unlock()
		s.setReportedURL(r.URL.RequestURI())
		_, cancel := context.WithCancel(context.Background())
		var mu sync.Mutex
		s.allTopics = true
		c := newConn(cancel)
		c.finish()
		s.attach(c)
		s.mu.Lock()
		s.connected = true
		s.mu.Unlock()
		for _, fn := range onConnect {
			fn(s)
		}
		sent := latestFilter{} // as the stream writer filters
		current := func() *connState {
			mu.Lock()
			defer mu.Unlock()
			return c
		}
		return livehook.Conn{
			Session: s,
			Drain: func() []livehook.Event {
				conn := current()
				var out []livehook.Event
				for {
					select {
					case ev := <-conn.inbox:
						if ev = s.take(conn, ev); !sent.newer(ev) {
							continue
						}
						out = append(out, livehook.Event{Kind: ev.kind, Topic: ev.topic, Body: ev.body})
					default:
						return out
					}
				}
			},
			Close: func() {
				cancel()
				s.detach(current())
				s.retire()
			},
			Subscribe: func(held map[string]int64) error { return s.subscribe(held, true) },
			Drop:      func() { s.detach(current()) },
			Resume: func() bool {
				_, cancel := context.WithCancel(context.Background())
				next := newConn(cancel)
				next.finish()
				_, stale, _, retired := s.attach(next)
				if retired || stale {
					return false
				}
				mu.Lock()
				c = next
				mu.Unlock()
				w.mu.RLock()
				onResume := slices.Clone(w.onResume)
				w.mu.RUnlock()
				for _, fn := range onResume {
					fn(s)
				}
				return true
			},
		}
	}
	livehook.Dispatch = func(sa any, action string, args jsontext.Value, form url.Values, r *http.Request) (any, error) {
		s := sa.(*Session)
		s.world.mu.RLock()
		fn, ok := s.world.commands[action]
		s.world.mu.RUnlock()
		if !ok {
			return nil, &MissingCommandError{Action: action}
		}
		info := s.world.captureRequest(r, s.params)
		s.mu.Lock()
		s.cmdReq = info
		s.mu.Unlock()
		in := jsonArgs(args)
		if form != nil {
			in = formArgs(form)
		}
		res, err := fn(s, in)
		// as the command endpoint does: the session is where the result declares
		if push, ok := res.(Push); ok && err == nil {
			s.setReportedURL(push.URL())
		}
		return res, err
	}
	livehook.CellValue = func(sa any, topic string) (jsontext.Value, bool) {
		s := sa.(*Session)
		s.cellMu.Lock()
		c, ok := s.cells[topic]
		s.cellMu.Unlock()
		if !ok {
			s.world.mu.RLock()
			c, ok = s.world.cells[topic]
			s.world.mu.RUnlock()
		}
		if !ok {
			return nil, false
		}
		body, _ := c.snapshot()
		return body, body != nil
	}
}
