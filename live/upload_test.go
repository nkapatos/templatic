package live

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionFor(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	r := openStream(t, ts, "up-sid")
	defer r.Close()
	r.discardUntilReady(t)

	if s := w.SessionFor(httptest.NewRequest("POST", "/import", nil)); s != nil {
		t.Fatal("no header resolved a session")
	}
	req := httptest.NewRequest("POST", "/import", nil)
	req.Header.Set(SidHeader, "nope")
	if s := w.SessionFor(req); s != nil {
		t.Fatal("unknown sid resolved a session")
	}
	if (*World)(nil).SessionFor(req) != nil || w.SessionFor(nil) != nil {
		t.Fatal("nil inputs resolved a session")
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "a.csv")
	_, _ = fw.Write([]byte("id,title\n1,x\n"))
	_ = mw.Close()
	req = httptest.NewRequest("POST", "/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(SidHeader, "up-sid")
	req.Header.Set("User-Agent", "probe")
	s := w.SessionFor(req)
	if s == nil || s.SID() != "up-sid" {
		t.Fatalf("SessionFor = %v", s)
	}
	if got := s.CommandRequest().Header.Get("User-Agent"); got != "probe" {
		t.Fatalf("CommandRequest UA = %q", got)
	}
	f, _, err := req.FormFile("file")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(f); string(b) != "id,title\n1,x\n" {
		t.Fatalf("file bytes = %q", b)
	}
}

func TestWriteInvalid(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteInvalid(rec, "file", "too large")
	res := rec.Result()
	if e := errorBody(t, res); e.Fields["file"] != "too large" {
		t.Fatalf("envelope = %+v", e)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatal(res.StatusCode)
	}
}
