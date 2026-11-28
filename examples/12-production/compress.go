package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// minGzipSize is the body size below which gzip costs more than it saves.
const minGzipSize = 512

var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

// gzipHandler gzips next's responses for clients that accept it, except
// event streams, non-200 responses and already compressed types.
func gzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if r.Method == http.MethodHead || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w, status: http.StatusOK}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

func acceptsGzip(accept string) bool {
	for part := range strings.SplitSeq(accept, ",") {
		coding, params, _ := strings.Cut(part, ";")
		if c := strings.TrimSpace(coding); (c == "gzip" || c == "*") && strings.TrimSpace(params) != "q=0" {
			return true
		}
	}
	return false
}

// gzipWriter buffers the first bytes and decides at the first Write past
// minGzipSize, at Flush, or at close, whichever comes first.
type gzipWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	decided     bool
	gz          *gzip.Writer // nil when the response passes through
	head        []byte
}

// Unwrap lets http.ResponseController reach the connection (write deadlines).
func (gw *gzipWriter) Unwrap() http.ResponseWriter { return gw.ResponseWriter }

func (gw *gzipWriter) WriteHeader(status int) {
	if gw.wroteHeader {
		return
	}
	gw.wroteHeader, gw.status = true, status
	if status != http.StatusOK {
		gw.passThrough()
	}
}

func (gw *gzipWriter) Write(p []byte) (int, error) {
	gw.WriteHeader(http.StatusOK)
	if gw.decided {
		if gw.gz != nil {
			return gw.gz.Write(p)
		}
		return gw.ResponseWriter.Write(p)
	}
	if !gw.compressible() {
		gw.passThrough()
		return gw.ResponseWriter.Write(p)
	}
	gw.head = append(gw.head, p...)
	if len(gw.head) >= minGzipSize {
		if err := gw.startGzip(); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush decides now, so a streamed response is never held back.
func (gw *gzipWriter) Flush() {
	if !gw.decided {
		if gw.compressible() {
			_ = gw.startGzip()
		} else {
			gw.passThrough()
		}
	}
	if gw.gz != nil {
		_ = gw.gz.Flush()
	}
	_ = http.NewResponseController(gw.ResponseWriter).Flush()
}

// compressible is false for event streams (each frame must reach the
// client as it is written) and for types that are compressed already.
func (gw *gzipWriter) compressible() bool {
	h := gw.Header()
	if h.Get("Content-Encoding") != "" {
		return false
	}
	ct := strings.ToLower(h.Get("Content-Type"))
	switch {
	case ct == "", strings.HasPrefix(ct, "text/event-stream"):
		return false
	case strings.HasPrefix(ct, "text/"), strings.HasPrefix(ct, "image/svg+xml"),
		strings.HasPrefix(ct, "application/json"), strings.HasPrefix(ct, "application/javascript"),
		strings.HasPrefix(ct, "application/manifest+json"):
		return true
	}
	return false
}

func (gw *gzipWriter) passThrough() {
	if gw.decided {
		return
	}
	gw.decided = true
	gw.ResponseWriter.WriteHeader(gw.status)
	if len(gw.head) > 0 {
		_, _ = gw.ResponseWriter.Write(gw.head)
		gw.head = nil
	}
}

func (gw *gzipWriter) startGzip() error {
	gw.decided = true
	h := gw.Header()
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	// The gzipped body is another representation: byte ranges and strong ETags no longer apply.
	h.Del("Accept-Ranges")
	if et := h.Get("ETag"); et != "" && !strings.HasPrefix(et, "W/") {
		h.Set("ETag", "W/"+et)
	}
	gw.ResponseWriter.WriteHeader(gw.status)
	gw.gz = gzipPool.Get().(*gzip.Writer)
	gw.gz.Reset(gw.ResponseWriter)
	_, err := gw.gz.Write(gw.head)
	gw.head = nil
	return err
}

func (gw *gzipWriter) close() {
	if !gw.decided {
		if !gw.wroteHeader && len(gw.head) == 0 {
			return // nothing written: net/http sends its own empty 200
		}
		gw.passThrough()
		return
	}
	if gw.gz != nil {
		_ = gw.gz.Close()
		gzipPool.Put(gw.gz)
		gw.gz = nil
	}
}
