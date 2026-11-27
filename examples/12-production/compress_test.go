package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzipHandler(t *testing.T) {
	html := strings.Repeat("<p>hello</p>", 100)
	tests := []struct {
		name     string
		method   string
		accept   string
		status   int
		ctype    string
		body     string
		wantGzip bool
	}{
		{"html", "GET", "gzip, br", 200, "text/html; charset=utf-8", html, true},
		{"small html", "GET", "gzip", 200, "text/html", "<p>hi</p>", false},
		{"no accept", "GET", "", 200, "text/html", html, false},
		{"refused", "GET", "gzip;q=0", 200, "text/html", html, false},
		{"head", "HEAD", "gzip", 200, "text/html", "", false},
		{"not modified", "GET", "gzip", 304, "text/html", "", false},
		{"no content", "GET", "gzip", 204, "", "", false},
		{"error page", "GET", "gzip", 404, "text/html", html, false},
		{"image", "GET", "gzip", 200, "image/png", html, false},
		{"event stream", "GET", "gzip", 200, "text/event-stream", html, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := gzipHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.ctype != "" {
					w.Header().Set("Content-Type", tt.ctype)
				}
				w.Header().Set("Content-Length", "1")
				w.WriteHeader(tt.status)
				if r.Method != "HEAD" {
					_, _ = io.WriteString(w, tt.body)
				}
			}))
			r := httptest.NewRequest(tt.method, "/", nil)
			if tt.accept != "" {
				r.Header.Set("Accept-Encoding", tt.accept)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)

			if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary = %q, want Accept-Encoding", got)
			}
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			gotGzip := rec.Header().Get("Content-Encoding") == "gzip"
			if gotGzip != tt.wantGzip {
				t.Fatalf("gzip = %v, want %v", gotGzip, tt.wantGzip)
			}
			body := rec.Body.String()
			if gotGzip {
				if cl := rec.Header().Get("Content-Length"); cl != "" {
					t.Errorf("Content-Length = %q on a gzipped response", cl)
				}
				zr, err := gzip.NewReader(rec.Body)
				if err != nil {
					t.Fatal(err)
				}
				b, err := io.ReadAll(zr)
				if err != nil {
					t.Fatal(err)
				}
				body = string(b)
			}
			if tt.method != "HEAD" && body != tt.body {
				t.Errorf("body = %q, want %q", body, tt.body)
			}
		})
	}
}

// An event stream's first frame reaches the client at Flush, uncompressed.
func TestGzipHandlerFlushesEventStream(t *testing.T) {
	frame := "data: 1\n\n"
	srv := httptest.NewServer(gzipHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, frame)
		http.NewResponseController(w).Flush()
		<-r.Context().Done()
	})))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ce := res.Header.Get("Content-Encoding"); ce != "" {
		t.Fatalf("Content-Encoding = %q, want none", ce)
	}
	buf := make([]byte, len(frame))
	if _, err := io.ReadFull(res.Body, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != frame {
		t.Fatalf("frame = %q, want %q", buf, frame)
	}
}
