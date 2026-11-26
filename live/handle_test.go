package live

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"net/http"
	"strings"
	"testing"
)

type captureIn struct {
	Title string `json:"title"`
	Count int    `json:"count"`
}

func (c captureIn) Validate() error {
	if strings.TrimSpace(c.Title) == "" {
		return Invalid("title", "a title is required")
	}
	return nil
}

func errorBody(t *testing.T, resp *http.Response) inputEnvelope {
	t.Helper()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Error *inputEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Error == nil {
		t.Fatalf("body %q is not the error envelope: %v", raw, err)
	}
	return *env.Error
}

func TestHandleInputErrors(t *testing.T) {
	w := NewWorld()
	var ran []captureIn
	w.Command("note.capture", Handle(func(s *Session, in captureIn) (any, error) {
		ran = append(ran, in)
		return map[string]int{"n": in.Count}, nil
	}))
	w.Command("note.raw", func(s *Session, args Args) (any, error) {
		return nil, Invalid("body", "too long")
	})
	w.Command("note.gone", Handle(func(s *Session, in captureIn) (any, error) {
		return nil, NotFound("no such note")
	}))
	w.Command("note.boom", Handle(func(s *Session, in captureIn) (any, error) {
		return nil, io.ErrUnexpectedEOF
	}))
	ts := serveWorld(t, w)
	r := openStream(t, ts, "h-sid")
	defer r.Close()
	r.discardUntilReady(t)

	if e := errorBody(t, postCommand(t, ts, "h-sid", "note.capture", jsontext.Value(`{"title":"x","count":"seven"}`))); e.Fields["count"] == "" {
		t.Fatalf("decode error = %+v", e)
	}
	if e := errorBody(t, postCommand(t, ts, "h-sid", "note.capture", jsontext.Value(`{"title":"  "}`))); e.Fields["title"] != "a title is required" {
		t.Fatalf("validate error = %+v", e)
	}
	if e := errorBody(t, postCommand(t, ts, "h-sid", "note.raw", nil)); e.Fields["body"] != "too long" {
		t.Fatalf("invalid error = %+v", e)
	}
	if len(ran) != 0 {
		t.Fatalf("handler ran on bad input: %v", ran)
	}
	resp := postCommand(t, ts, "h-sid", "note.boom", jsontext.Value(`{"title":"ok"}`))
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusInternalServerError || strings.Contains(string(raw), "EOF") {
		t.Fatalf("500 leaked detail or wrong status: %d %q", resp.StatusCode, raw)
	}
	resp = postCommand(t, ts, "h-sid", "note.gone", jsontext.Value(`{"title":"ok"}`))
	raw, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(raw), "no such note") {
		t.Fatalf("status error: %d %q", resp.StatusCode, raw)
	}
	resp = postCommand(t, ts, "h-sid", "note.capture", jsontext.Value(`{"title":"hi","count":3}`))
	raw, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"n":3`) || len(ran) != 1 {
		t.Fatalf("success path: %d %q %v", resp.StatusCode, raw, ran)
	}
}
