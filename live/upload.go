package live

import "net/http"

// SidHeader names the header carrying the session id on a form POST the runtime sends.
const SidHeader = "X-TC-Sid"

// SessionFor returns the live session named by the request's SidHeader, or nil.
func (w *World) SessionFor(r *http.Request) *Session {
	if w == nil || r == nil {
		return nil
	}
	sid := r.Header.Get(SidHeader)
	if sid == "" {
		return nil
	}
	return w.sessionByID(sid, r)
}

// sessionByID returns the live session sid with r recorded as its command request, or nil.
func (w *World) sessionByID(sid string, r *http.Request) *Session {
	w.mu.RLock()
	s, ok := w.sessions[sid]
	w.mu.RUnlock()
	if !ok || s.isRetired() {
		return nil
	}
	info := w.captureRequest(r, s.params)
	s.mu.Lock()
	s.cmdReq = info
	s.mu.Unlock()
	return s
}

// WriteInvalid answers a refused field with the 400 input envelope.
func WriteInvalid(w http.ResponseWriter, field, message string) {
	WriteInputError(w, &InputError{Field: field, Message: message})
}
