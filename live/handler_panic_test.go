package live

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPanicInConnectCallbackDetachesTheConnection(t *testing.T) {
	w := NewWorld(WithResumeGrace(0))
	w.OnConnect(func(*Session) { panic("boom") })
	h := &streamHandler{world: w}
	req := httptest.NewRequest("GET", "/live/stream?t="+url.QueryEscape(tokenFor(w, "pp1")), nil)

	func() {
		defer func() { _ = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), req)
	}()
	if s := sessionOf(w, "pp1"); s != nil && s.Attached() {
		t.Fatal("panicking OnConnect leaked an attached connection")
	}
}
