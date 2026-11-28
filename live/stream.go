package live

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nkapatos/templatic/internal/markup"
)

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

type streamHandler struct{ world *World }

// A session is reattached only by a token it issued, so a guessed sid cannot take it over.
func authorizeReattach(s *Session, token string) bool { return s.knowsToken(token) }

func (h *streamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sid, token := q.Get("sid"), q.Get("t")
	var params map[string]string
	var seeds map[string]jsontext.Value

	// The controller follows Unwrap, so app middleware that wraps the writer without Flush still streams.
	rc := http.NewResponseController(w)
	if !canFlush(w) {
		http.Error(w, "live: streaming unsupported", http.StatusInternalServerError)
		return
	}

	// ?r=1 marks the runtime's manual reconnect, which lets ?u= follow live.At.
	reconnect := r.Header.Get("Last-Event-ID") != "" || q.Get("r") != ""

	// A reconnect cannot see HTTP statuses (EventSource would retry forever), so token failures resync instead.
	reject := func(code int, msg, reason string) {
		if reconnect {
			h.resyncOnly(w, rc, reason)
			return
		}
		http.Error(w, msg, code)
	}

	if token == "" {
		reject(http.StatusBadRequest, "live: page token required", ReasonLost)
		return
	}
	// An expired token is authentic; it admits only a reattach to the live session it minted.
	expired := false
	claims, err := h.world.verifyToken(token)
	if errors.Is(err, errTokenExpired) {
		expired = true
	} else if err != nil {
		reject(http.StatusForbidden, err.Error(), ReasonLost)
		return
	}
	if sid != "" && sid != claims.SID {
		reject(http.StatusBadRequest, "live: sid does not match page token", ReasonLost)
		return
	}
	sid, params, seeds = claims.SID, claims.Params, claims.Seeds
	if h.world.admit != nil && !h.world.admit(r, maps.Clone(params)) {
		h.world.mu.RLock()
		s, known := h.world.sessions[sid]
		h.world.mu.RUnlock()
		if known && authorizeReattach(s, token) {
			s.end(ReasonRefused)
		}
		reject(http.StatusForbidden, "live: connect refused", ReasonRefused)
		return
	}

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
			reject(http.StatusForbidden, errTokenExpired.Error(), ReasonLost)
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
				h.resyncOnly(w, rc, ReasonLost)
				return
			}
			s = &Session{world: h.world, sid: sid, token: token, params: params, created: h.world.clock()}
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
				h.resyncOnly(w, rc, ReasonLost)
				return
			}
			continue
		}
		break
	}
	defer s.detach(c) // runs on every exit, including a panicking callback
	if stale {
		s.retire()
		h.resyncOnly(w, rc, ReasonLost)
		return
	}
	if maxAge := h.world.maxAge; maxAge > 0 {
		left := s.created.Add(maxAge).Sub(h.world.clock())
		if left <= 0 {
			s.retire()
			h.resyncOnly(w, rc, ReasonMaxAge)
			return
		}
		t := time.AfterFunc(left, func() { s.end(ReasonMaxAge) })
		defer t.Stop()
	}
	// Each attach hands the page a fresh token, so a token seen in a URL (EventSource cannot send headers) stops
	// working two attaches later.
	if next := s.rotateToken(); next != "" {
		prelude = append([]event{{kind: KindToken, body: tokenBody(next)}}, prelude...)
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
	// Fresh or not, the page says which collections it shows and at what version, so a gap since its render is
	// caught up too. A collection that cannot be rendered is skipped: a broken frame would not be repaired.
	_ = s.subscribe(parseHeld(q.Get("c")))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()

	// Frame writes carry a deadline so a vanished client cannot leave writes buffering forever; test doubles skip it.
	deadline := h.world.writeDeadline()
	setDeadline := func() {
		if deadline <= 0 {
			return
		}
		_ = rc.SetWriteDeadline(time.Now().Add(deadline))
	}
	writeWithDeadline := func(p []byte) (int, error) {
		setDeadline()
		return w.Write(p)
	}

	// The loop flushes once per batch of frames, never holding a frame past its batch.
	_ = s.loop(ctx, c, prelude, writerFunc(writeWithDeadline), func() {
		setDeadline()
		_ = rc.Flush()
	})
}

// canFlush reports whether w, or a writer it unwraps to, implements http.Flusher.
func canFlush(w http.ResponseWriter) bool {
	for {
		if _, ok := w.(http.Flusher); ok {
			return true
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = u.Unwrap()
	}
}

func (h *streamHandler) resyncOnly(w http.ResponseWriter, rc *http.ResponseController, reason string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_ = markup.WriteStrings(w, "id: 1\nevent: tc\ndata: {\"kind\":\"resync\",\"reason\":\"", reason, "\"}\n\n")
	_ = rc.Flush()
}

// parseHeld reads "topic:ver,topic:ver"; a malformed pair is skipped, and a missing version reads as -1 (none).
func parseHeld(raw string) map[string]int64 {
	if raw == "" {
		return nil
	}
	out := map[string]int64{}
	for pair := range strings.SplitSeq(raw, ",") {
		topic, ver, _ := strings.Cut(pair, ":")
		if checkTopic(topic) != nil {
			continue
		}
		n, err := strconv.ParseInt(ver, 10, 64)
		if err != nil || n < 0 {
			n = -1
		}
		out[topic] = n
	}
	return out
}
