package live

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a-h/templ"
)

func TestCellSetPublishesToSessions(t *testing.T) {
	w := NewWorld()
	tick := NewCell(w, "tick", int64(1))

	ts := serveWorld(t, w)
	r := openStream(t, ts, "s1")
	defer r.Close()

	if got, ok := r.next(t, 2*time.Second); !ok || !strings.Contains(got, `"kind":"cell"`) || !strings.Contains(got, `"v":1`) {
		t.Fatalf("missing initial cell frame: %q", got)
	}
	ready, _ := r.next(t, 2*time.Second)
	if !strings.Contains(ready, `"kind":"ready"`) {
		t.Fatalf("missing ready frame, got %q", ready)
	}

	tick.Set(42)

	frame, ok := r.next(t, 2*time.Second)
	if !ok || !strings.Contains(frame, `"v":42`) {
		t.Fatalf("expected cell v 42, got %q ok=%v", frame, ok)
	}
}

func TestInitialSnapshotCarriesCellValues(t *testing.T) {
	w := NewWorld()
	NewCell(w, "tick", int64(7))

	ts := serveWorld(t, w)
	r := openStream(t, ts, "s2")
	defer r.Close()

	initial, _ := r.next(t, time.Second)
	if !strings.Contains(initial, `"v":7`) {
		t.Fatalf("expected initial snapshot v 7, got %q", initial)
	}
}

func TestCoalescingKeepsLatestPerTopic(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	r := openStream(t, ts, "s3")
	defer r.Close()
	r.discardUntilReady(t)

	tick := NewCell(w, "tick", int64(0))
	for i := int64(1); i <= 5; i++ {
		tick.Set(i)
	}

	var cellFrames []string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		frame, ok := r.next(t, 200*time.Millisecond)
		if !ok {
			break
		}
		if strings.Contains(frame, `"kind":"cell"`) {
			cellFrames = append(cellFrames, frame)
			if strings.Contains(frame, `"v":5`) {
				break
			}
		}
	}
	if len(cellFrames) == 0 {
		t.Fatal("no cell frames after burst")
	}
	for _, f := range cellFrames[:len(cellFrames)-1] {
		if !strings.Contains(f, `"v":5`) {
			t.Fatalf("intermediate value leaked through coalescing: %v", cellFrames)
		}
	}
	if !strings.Contains(cellFrames[len(cellFrames)-1], `"v":5`) {
		t.Fatalf("latest value not delivered: %v", cellFrames)
	}
}

func TestRedrawPushesRegionMarkup(t *testing.T) {
	w := NewWorld()
	tick := NewCell(w, "tick", int64(3))

	w.Region("feed", func() templ.Component {
		return templ.ComponentFunc(func(ctx context.Context, wr io.Writer) error {
			_, err := io.WriteString(wr, "<ul><li>item "+itoa(tick.Get())+"</li></ul>")
			return err
		})
	})

	ts := serveWorld(t, w)
	r := openStream(t, ts, "s4")
	defer r.Close()
	r.discardUntilReady(t)

	tick.Set(42)
	if err := w.Redraw("feed"); err != nil {
		t.Fatal(err)
	}

	var swapFrame string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		frame, ok := r.next(t, 200*time.Millisecond)
		if !ok {
			continue
		}
		if strings.Contains(frame, `"kind":"swap"`) {
			swapFrame = frame
			break
		}
	}
	if swapFrame == "" {
		t.Fatal("swap frame never arrived")
	}
	if !strings.Contains(swapFrame, `"topic":"feed"`) || !strings.Contains(swapFrame, "item 42") {
		t.Fatalf("swap frame wrong: %q", swapFrame)
	}
}

func TestRedrawUnknownRegionErrors(t *testing.T) {
	w := NewWorld()
	if err := w.Redraw("nope"); err == nil {
		t.Fatal("expected error for unknown region")
	}
}

func TestCommandsDispatchToHandler(t *testing.T) {
	w := NewWorld()
	tick := NewCell(w, "tick", int64(0))
	w.Command("tick.add", func(s *Session, args Args) (any, error) {
		if s.SID() != "cmd-sid" {
			t.Errorf("handler saw sid %q", s.SID())
		}
		var n int64
		if err := args.Into(&n); err != nil {
			return nil, err
		}
		tick.Set(tick.Get() + n)
		return nil, nil
	})

	ts := serveWorld(t, w)
	r := openStream(t, ts, "cmd-sid")
	defer r.Close()
	r.discardUntilReady(t)

	resp := postCommand(t, ts, "cmd-sid", "tick.add", jsontext.Value("5"))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	frame, ok := r.next(t, 2*time.Second)
	if !ok || !strings.Contains(frame, `"v":5`) {
		t.Fatalf("command effect not visible on stream: %q ok=%v", frame, ok)
	}
}

func TestCommandsReturnResultPayloads(t *testing.T) {
	w := NewWorld()
	w.Command("chat.nudge", func(s *Session, args Args) (any, error) {
		return nil, nil
	})
	w.Command("note.save", func(s *Session, args Args) (any, error) {
		var in struct {
			Version int `json:"expectedVersion"`
		}
		if err := args.Into(&in); err != nil {
			return nil, err
		}
		return map[string]any{"version": in.Version + 1, "conflict": false}, nil
	})

	ts := serveWorld(t, w)
	r := openStream(t, ts, "res-sid")
	defer r.Close()
	r.discardUntilReady(t)

	resp := postCommand(t, ts, "res-sid", "chat.nudge", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("nil result must stay 204, got %d", resp.StatusCode)
	}

	resp = postCommand(t, ts, "res-sid", "note.save", jsontext.Value(`{"expectedVersion":3}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("result dispatch must be 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("result envelope %s: %v", body, err)
	}
	if want := (map[string]any{"version": 4.0, "conflict": false}); !reflect.DeepEqual(env.Result, want) {
		t.Fatalf("expected result %v, got %s", want, body)
	}
}

func TestAtDeclaresPushURL(t *testing.T) {
	w := NewWorld()
	w.Command("daily.open", func(s *Session, args Args) (any, error) {
		return At("/daily/2026-01-05"), nil
	})
	w.Command("note.open", func(s *Session, args Args) (any, error) {
		return At("/workspace/nt-carry"), nil
	})

	ts := serveWorld(t, w)
	r := openStream(t, ts, "at-sid")
	defer r.Close()
	r.discardUntilReady(t)

	resp := postCommand(t, ts, "at-sid", "daily.open", jsontext.Value(`{"day":"2026-01-05"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("At dispatch must be 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"url":"/daily/2026-01-05"}`
	if strings.TrimSpace(string(body)) != want {
		t.Fatalf("expected url envelope %s, got %s", want, string(body))
	}

	resp = postCommand(t, ts, "at-sid", "note.open", nil)
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"/workspace/nt-carry"`) {
		t.Fatalf("note.open url missing: %s", string(body))
	}
	if got := sessionOf(w, "at-sid").URL(); got.Path != "/workspace/nt-carry" {
		t.Fatalf("Session.URL() must follow At, got %v", got)
	}
}

func TestAtRejectsNonRelativeURL(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("At must panic on an absolute URL (client-side push of a cross-origin address is forbidden by construction)")
		}
	}()
	_ = At("https://evil.example/daily")
}

func TestAtRejectsSchemelessHost(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("At must panic on scheme// host forms")
		}
	}()
	_ = At("//evil.example/daily")
}

func TestCommandsRejectUnknownSession(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	resp := postCommand(t, ts, "missing", "act", jsontext.Value(`{}`))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for unknown session, got %d", resp.StatusCode)
	}
}

func TestCommandsRejectUnknownAction(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	r := openStream(t, ts, "act-sid")
	defer r.Close()
	r.discardUntilReady(t)
	resp := postCommand(t, ts, "act-sid", "nope", jsontext.Value(`{}`))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown action, got %d", resp.StatusCode)
	}
}

func TestTopicValidation(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on malformed topic")
		}
	}()
	w := NewWorld()
	NewCell(w, "bad topic!", 0)
	_ = w
}

func serveWorld(t *testing.T, w *World) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	w.Mount(mux, "/live")
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	servedWorlds.Store(ts.URL, w)
	t.Cleanup(func() {
		servedWorlds.Delete(ts.URL)
	})
	return ts
}

var (
	servedWorlds sync.Map
	testTokens   sync.Map // tokenKey → the page's newest token
)

// tokenKey holds the world itself, not its address, so a later world cannot find another's tokens.
type tokenKey struct {
	w   *World
	sid string
}

func tokenFor(w *World, sid string) string {
	key := tokenKey{w, sid}
	if tok, ok := testTokens.Load(key); ok {
		return tok.(string)
	}
	payload, err := json.Marshal(tokenClaims{SID: sid, Issued: w.clock().Unix()})
	if err != nil {
		panic(err)
	}
	tok, _ := testTokens.LoadOrStore(key, w.sign(payload))
	return tok.(string)
}

type stream struct {
	body     io.Closer
	sc       *bufio.Scanner
	tokenKey any // where the page's newest token is kept, as the client keeps it
}

func (s *stream) Close() { _ = s.body.Close() }

// latestToken is the newest token the stream has sent so far, or "".
func (s *stream) latestToken() string {
	v, _ := testTokens.Load(s.tokenKey)
	tok, _ := v.(string)
	return tok
}

func openStream(t *testing.T, ts *httptest.Server, sid string) *stream {
	t.Helper()
	return openStreamWithHeaders(t, ts, sid, nil)
}

func openStreamWithHeaders(t *testing.T, ts *httptest.Server, sid string, headers map[string]string) *stream {
	t.Helper()
	return openStreamQuery(t, ts, sid, "", headers)
}

// openStreamQuery opens sid's stream with extra appended to the query, such as "c=feed:3".
func openStreamQuery(t *testing.T, ts *httptest.Server, sid, extra string, headers map[string]string) *stream {
	t.Helper()
	wv, _ := servedWorlds.Load(ts.URL)
	wOf := wv.(*World)
	q := "t=" + url.QueryEscape(tokenFor(wOf, sid))
	if extra != "" {
		q += "&" + extra
	}
	req, err := http.NewRequest("GET", ts.URL+"/live/stream?"+q, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", resp.StatusCode)
	}
	t.Cleanup(func() { resp.Body.Close() })
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return &stream{body: resp.Body, sc: sc, tokenKey: tokenKey{wOf, sid}}
}

func (s *stream) next(t *testing.T, timeout time.Duration) (string, bool) {
	t.Helper()
	type res struct {
		frame string
		ok    bool
	}
	ch := make(chan res, 1)
	go func() {
		var lines []string
		for s.sc.Scan() {
			line := s.sc.Text()
			if line == "" {
				if len(lines) > 0 {
					frame := strings.Join(lines, "\n")
					if tok, ok := tokenOf(frame); ok {
						testTokens.Store(s.tokenKey, tok)
						lines = nil
						continue
					}
					ch <- res{frame, true}
					return
				}
				continue
			}
			lines = append(lines, line)
		}
		ch <- res{"", s.sc.Err() == nil}
	}()
	select {
	case got := <-ch:
		return got.frame, got.ok
	case <-time.After(timeout):
		return "", false
	}
}

func (s *stream) discardUntilReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		frame, ok := s.next(t, time.Until(deadline))
		if !ok {
			t.Fatal("ready frame never arrived")
		}
		if strings.Contains(frame, `"kind":"ready"`) {
			return
		}
	}
}

func postCommand(t *testing.T, ts *httptest.Server, sid, action string, args jsontext.Value) *http.Response {
	t.Helper()
	payload := map[string]any{"sid": sid, "action": action}
	if args != nil {
		payload["args"] = args
	}
	buf, _ := json.Marshal(payload)
	resp, err := http.Post(ts.URL+"/live/commands", "application/json", strings.NewReader(string(buf)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// tokenOf returns the token a token frame carries.
func tokenOf(frame string) (string, bool) {
	_, data, ok := strings.Cut(frame, "data: ")
	if !ok || !strings.Contains(data, `"kind":"token"`) {
		return "", false
	}
	var v struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return "", false
	}
	return v.Token, true
}
