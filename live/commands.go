package live

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
)

// CommandHandler runs an action raised from the page; a non-nil result is sent to
// the client, nil answers 204, and At(url) moves the address.
type CommandHandler func(s *Session, args Args) (any, error)

// MissingCommandError reports an action with no registered handler.
type MissingCommandError struct{ Action string }

func (e *MissingCommandError) Error() string {
	return "live: no handler for action " + e.Action
}

const maxCommandBytes = 1 << 20

func tooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

type commandsHandler struct{ world *World }

func (h *commandsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req commandRequest
	if err := json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxCommandBytes), &req); err != nil {
		if tooLarge(err) {
			http.Error(w, "live: command payload over 1 MiB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "live: bad command payload", http.StatusBadRequest)
		return
	}
	if req.SID != "" && req.Action == "" && len(req.Subscribe) > 0 {
		h.subscribe(w, req)
		return
	}
	if req.SID == "" || req.Action == "" {
		http.Error(w, "live: sid and action required", http.StatusBadRequest)
		return
	}
	args := jsonArgs(req.Args)
	if req.Form != nil {
		vals, err := url.ParseQuery(*req.Form)
		if err != nil || len(req.Args) != 0 {
			http.Error(w, "live: bad command payload", http.StatusBadRequest)
			return
		}
		args = formArgs(vals)
	}

	h.world.mu.RLock()
	s, sessionOK := h.world.sessions[req.SID]
	fn, commandOK := h.world.commands[req.Action]
	h.world.mu.RUnlock()

	if !sessionOK {
		http.Error(w, "live: unknown session; (re)connect the stream first",
			http.StatusConflict)
		return
	}
	if !commandOK {
		http.Error(w, (&MissingCommandError{Action: req.Action}).Error(),
			http.StatusNotFound)
		return
	}

	if c, ok := h.world.verifySubmit(req.Submit); ok && c.SID == s.sid {
		w.Header().Set(SubmitHeader, h.world.mintSubmit(s.sid))
		sub, first := h.world.claimSubmit(c)
		if !first {
			sub.replayJS(w)
			return
		}
		tee := &responseTee{ResponseWriter: w}
		settled := false
		defer func() {
			if !settled {
				sub.settle(false)
			}
		}()
		h.run(tee, r, s, fn, args)
		sub.js, sub.status, sub.ctype, sub.body = true, tee.status, tee.Header().Get("Content-Type"), tee.body.Bytes()
		settled = true
		sub.settle(tee.status >= 200 && tee.status < 300)
		return
	}
	h.run(w, r, s, fn, args)
}

func (h *commandsHandler) run(w http.ResponseWriter, r *http.Request, s *Session, fn CommandHandler, args Args) {
	info := h.world.captureRequest(r, s.params)
	s.mu.Lock()
	s.cmdReq = info
	s.mu.Unlock()

	result, err := fn(s, args)
	if err != nil {
		if WriteInputError(w, err) {
			return
		}
		var mce *MissingCommandError
		if errors.As(err, &mce) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if writeStatusError(w, err) {
			return
		}
		// Unclassified errors are not exposed to the client.
		http.Error(w, "live: command failed", http.StatusInternalServerError)
		return
	}
	if result == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if push, ok := result.(Push); ok {
		// The client pushes this URL on success, so it is the page's address from now on.
		s.setReportedURL(push.URL())
		payload, err := json.Marshal(struct {
			URL string `json:"url"`
		}{URL: push.URL()})
		if err != nil {
			http.Error(w, "live: command result is not encodable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(append(payload, '\n')); err != nil {
			return
		}
		return
	}
	payload, err := json.Marshal(struct {
		Result any `json:"result"`
	}{Result: result})
	if err != nil {
		http.Error(w, "live: command result is not encodable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(append(payload, '\n')); err != nil {
		return
	}
}

// subscribe answers 204 at once; what the page lacks arrives on its stream, after the frames already queued.
func (h *commandsHandler) subscribe(w http.ResponseWriter, req commandRequest) {
	h.world.mu.RLock()
	s, ok := h.world.sessions[req.SID]
	h.world.mu.RUnlock()
	if !ok || s.isRetired() {
		http.Error(w, "live: unknown session; (re)connect the stream first", http.StatusConflict)
		return
	}
	if err := s.subscribe(req.Subscribe, true); err != nil {
		http.Error(w, "live: subscribe failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
