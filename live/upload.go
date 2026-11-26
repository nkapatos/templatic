package live

import "net/http"

// SidHeader names the header carrying the session id on a JS multipart upload.
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
	w.mu.RLock()
	s, ok := w.sessions[sid]
	w.mu.RUnlock()
	if !ok {
		return nil
	}
	info := w.captureRequest(r, s.params)
	s.mu.Lock()
	s.cmdReq = info
	s.mu.Unlock()
	return s
}

// WriteInvalid answers a refused upload with the 400 input envelope.
func WriteInvalid(w http.ResponseWriter, field, message string) {
	WriteInputError(w, &InputError{Field: field, Message: message})
}
