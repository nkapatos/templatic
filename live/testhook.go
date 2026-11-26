package live

import (
	"context"
	"encoding/json/jsontext"
	"net/http"
	"net/url"
	"slices"

	"github.com/nkapatos/templatic/internal/livehook"
)

func init() {
	livehook.Connect = func(wa any, sid string, r *http.Request) livehook.Conn {
		w := wa.(*World)
		s := &Session{world: w, sid: sid}
		w.mu.Lock()
		w.sessions[sid] = s
		onConnect := slices.Clone(w.onConnect)
		w.mu.Unlock()
		info := w.captureRequest(r, nil)
		s.mu.Lock()
		s.req = info
		s.mu.Unlock()
		s.setReportedURL(r.URL.RequestURI())
		_, cancel := context.WithCancel(context.Background())
		c := newConn(cancel)
		c.finish()
		s.attach(c)
		s.mu.Lock()
		s.connected = true
		s.mu.Unlock()
		for _, fn := range onConnect {
			fn(s)
		}
		return livehook.Conn{
			Session: s,
			Drain: func() []livehook.Event {
				var out []livehook.Event
				for {
					select {
					case ev := <-c.inbox:
						out = append(out, livehook.Event{Kind: ev.kind, Topic: ev.topic, Body: ev.body})
					default:
						return out
					}
				}
			},
			Close: func() {
				cancel()
				s.detach(c)
				s.retire()
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
		if form != nil {
			return fn(s, formArgs(form))
		}
		return fn(s, jsonArgs(args))
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
		body := c.snapshot()
		return body, body != nil
	}
}
