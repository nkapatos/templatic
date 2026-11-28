package live

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// unwrapOnly wraps a writer the way app middleware does since Go 1.20: Unwrap, no Flush.
type unwrapOnly struct{ http.ResponseWriter }

func (u unwrapOnly) Unwrap() http.ResponseWriter { return u.ResponseWriter }

// plainWriter hides every optional interface of the writer it wraps.
type plainWriter struct{ w http.ResponseWriter }

func (p plainWriter) Header() http.Header         { return p.w.Header() }
func (p plainWriter) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p plainWriter) WriteHeader(code int)        { p.w.WriteHeader(code) }

func serveWrapped(t *testing.T, w *World, wrap func(http.ResponseWriter) http.ResponseWriter) *httptest.Server {
	t.Helper()
	w.OnConnect(func(s *Session) {
		s.topicsMu.Lock()
		s.allTopics = true
		s.topicsMu.Unlock()
	})
	mux := http.NewServeMux()
	w.Mount(mux, "/live")
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(wrap(rw), r)
	}))
	t.Cleanup(ts.Close)
	servedWorlds.Store(ts.URL, w)
	t.Cleanup(func() { servedWorlds.Delete(ts.URL) })
	return ts
}

func TestStreamFlushesThroughUnwrapOnlyWriter(t *testing.T) {
	w := NewWorld()
	tick := NewCell(w, "tick", int64(1))
	ts := serveWrapped(t, w, func(rw http.ResponseWriter) http.ResponseWriter { return unwrapOnly{rw} })
	r := openStream(t, ts, "s1")
	defer r.Close()
	r.discardUntilReady(t)

	tick.Set(2)
	if frame, ok := r.next(t, 2*time.Second); !ok || !strings.Contains(frame, `"v":2`) {
		t.Fatalf("frame not flushed through the wrapper: %q ok=%v", frame, ok)
	}
}

func TestStreamRefusesWriterThatCannotFlush(t *testing.T) {
	w := NewWorld()
	ts := serveWrapped(t, w, func(rw http.ResponseWriter) http.ResponseWriter { return plainWriter{rw} })
	resp, err := http.Get(ts.URL + "/live/stream?t=" + tokenFor(w, "s1"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", resp.StatusCode)
	}
}
