package live

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"slices"
	"time"
)

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

type streamHandler struct{ world *World }

// A session minted from a page token is reattached only by that token, so a guessed sid cannot take it over.
func authorizeReattach(s *Session, token string) bool {
	return token != "" && tokensEqual(s.token, token)
}

func (h *streamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sid, token := q.Get("sid"), q.Get("t")
	var params map[string]string
	var seeds map[string]jsontext.Value

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "live: streaming unsupported", http.StatusInternalServerError)
		return
	}

	// ?r=1 marks the runtime's manual reconnect, which lets ?u= follow live.At.
	reconnect := r.Header.Get("Last-Event-ID") != "" || q.Get("r") != ""

	// A reconnect cannot see HTTP statuses (EventSource would retry forever), so token failures resync instead.
	reject := func(code int, msg string) {
		if reconnect {
			h.resyncOnly(w, flusher)
			return
		}
		http.Error(w, msg, code)
	}

	if token == "" {
		reject(http.StatusBadRequest, "live: page token required")
		return
	}
	// An expired token is authentic; it admits only a reattach to the live session it minted.
	expired := false
	claims, err := h.world.verifyToken(token)
	if errors.Is(err, errTokenExpired) {
		expired = true
	} else if err != nil {
		reject(http.StatusForbidden, err.Error())
		return
	}
	if sid != "" && sid != claims.SID {
		reject(http.StatusBadRequest, "live: sid does not match page token")
		return
	}
	sid, params, seeds = claims.SID, claims.Params, claims.Seeds

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	c := newConn(cancel)
	defer c.finish()
	var (
		s       *Session
		prelude []event
		stale   bool
		resumed bool
	)
	for attempt := 0; ; attempt++ {
		h.world.mu.Lock()
		var known bool
		s, known = h.world.sessions[sid]
		if known && s.isRetired() {
			// Mid-retirement: treat as gone.
			known = false
		}
		if expired && !known {
			h.world.mu.Unlock()
			reject(http.StatusForbidden, errTokenExpired.Error())
			return
		}
		if known && !authorizeReattach(s, token) {
			h.world.mu.Unlock()
			http.Error(w, "live: session identity not proven", http.StatusForbidden)
			return
		}
		if !known {
			if reconnect {
				// The server no longer holds the session, so the page's DOM cannot be trusted: resync rather than start a blank one.
				h.world.mu.Unlock()
				h.resyncOnly(w, flusher)
				return
			}
			s = &Session{world: h.world, sid: sid, token: token, params: params}
			for topic, raw := range seeds {
				if s.cells == nil {
					s.cells = map[string]cellView{}
				}
				s.cells[topic] = &seedCell{raw: raw}
			}
			h.world.sessions[sid] = s
		}
		h.world.mu.Unlock()

		// The *http.Request itself is never kept.
		info := h.world.captureRequest(r, s.params)
		s.mu.Lock()
		s.req = info
		s.mu.Unlock()
		s.setReportedURL(q.Get("u"))

		var retired bool
		prelude, stale, resumed, retired = s.attach(c)
		if retired {
			if reconnect || attempt > 0 {
				h.resyncOnly(w, flusher)
				return
			}
			continue
		}
		break
	}
	defer s.detach(c) // runs on every exit, including a panicking callback
	if stale {
		s.retire()
		h.resyncOnly(w, flusher)
		return
	}

	h.world.mu.RLock()
	onConnect := slices.Clone(h.world.onConnect)
	onResume := slices.Clone(h.world.onResume)
	h.world.mu.RUnlock()
	s.mu.Lock()
	first := !s.connected
	s.connected = true
	s.mu.Unlock()
	if first {
		for _, fn := range onConnect {
			fn(s)
		}
	} else if resumed {
		for _, fn := range onResume {
			fn(s)
		}
	}
	// A fresh load needs no replay: its markup already holds the rendered state.
	if reconnect {
		h.world.replayStreams(s)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Frame writes carry a deadline so a vanished client cannot leave writes buffering forever; test doubles skip it.
	deadline := h.world.writeDeadline()
	setDeadline := func() {
		if deadline <= 0 {
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(deadline))
	}
	writeWithDeadline := func(p []byte) (int, error) {
		setDeadline()
		n, err := w.Write(p)
		if err != nil {
			return n, err
		}
		flusher.Flush()
		return n, nil
	}

	_ = s.loop(ctx, c, prelude, writerFunc(writeWithDeadline), func() {})
}

func (h *streamHandler) resyncOnly(w http.ResponseWriter, flusher http.Flusher) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("id: 1\nevent: tc\ndata: {\"kind\":\"resync\"}\n\n"))
	flusher.Flush()
}
