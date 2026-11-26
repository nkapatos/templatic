package live

import (
	"bufio"
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a-h/templ"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func connectStatus(t *testing.T, ts *httptest.Server, query string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", ts.URL+"/live/stream?"+query, nil)
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
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func openToken(t *testing.T, ts *httptest.Server, token string, headers map[string]string) *stream {
	t.Helper()
	return openToken2(t, ts, "t="+url.QueryEscape(token), headers)
}

func openToken2(t *testing.T, ts *httptest.Server, query string, headers map[string]string) *stream {
	t.Helper()
	resp := connectStatus(t, ts, query, headers)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token connect status %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	return &stream{body: resp.Body, sc: sc}
}

func claimsOf(t *testing.T, w *World, token string) tokenClaims {
	t.Helper()
	c, err := w.verifyToken(token)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPageTokenRoundTripsAndMintsDistinctSIDs(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	a := w.PageToken(nil, TokenParam("k", "v"))
	b := w.PageToken(nil)
	ca, cb := claimsOf(t, w, a), claimsOf(t, w, b)
	if ca.SID == "" || ca.SID == cb.SID {
		t.Fatalf("sids must be minted and distinct: %q %q", ca.SID, cb.SID)
	}
	if ca.Params["k"] != "v" {
		t.Fatalf("params lost: %+v", ca.Params)
	}
}

func TestBadTokensAre403(t *testing.T) {
	w := NewWorld(WithKey(testKey), WithTokenTTL(time.Hour))
	ts := serveWorld(t, w)
	good := w.PageToken(nil)
	body, sig, _ := strings.Cut(good, ".")

	other := NewWorld(WithKey([]byte("another-key-another-key-another!")))
	forged := other.PageToken(nil)

	tamperedPayload := other.PageToken(nil)
	tb, _, _ := strings.Cut(tamperedPayload, ".")

	old := NewWorld(WithKey(testKey), WithTokenTTL(time.Hour))
	old.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	expired := old.PageToken(nil)

	for name, tok := range map[string]string{
		"forged key":       forged,
		"tampered payload": tb + "." + sig,
		"tampered sig":     body + "." + sig[:len(sig)-2] + "AA",
		"expired":          expired,
		"garbage":          "nonsense",
		"no sig":           body + ".",
	} {
		resp := connectStatus(t, ts, "t="+url.QueryEscape(tok), nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, resp.StatusCode)
		}
	}
	if resp := connectStatus(t, ts, "t="+url.QueryEscape(good), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("valid token rejected: %d", resp.StatusCode)
	}
}

func TestTokenSessionCannotBeImpersonated(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	life := trackLifecycle(w)
	ts := serveWorld(t, w)
	tok := w.PageToken(nil)
	claims := claimsOf(t, w, tok)
	a := openToken(t, ts, tok, nil)
	readUntilReady(t, a)
	victim := sessionOf(w, claims.SID)
	if victim == nil {
		t.Fatal("session not registered under the minted sid")
	}

	if resp := connectStatus(t, ts, "sid="+claims.SID, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bare-sid connect status %d, want 400", resp.StatusCode)
	}
	forgedSame := w.sign(mustJSON(t, tokenClaims{SID: claims.SID, Issued: claims.Issued - 1}))
	if resp := connectStatus(t, ts, "t="+url.QueryEscape(forgedSame), nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("different-token takeover status %d, want 403", resp.StatusCode)
	}
	if resp := connectStatus(t, ts, "sid=other&t="+url.QueryEscape(tok), nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("sid/token mismatch status %d, want 400", resp.StatusCode)
	}
	if sessionOf(w, claims.SID) != victim || !victim.Attached() || life.get() != "connect" {
		t.Fatalf("rejected attempts disturbed the live session (%s)", life.get())
	}

	b := openToken(t, ts, tok, reconnectHeader)
	readUntilReady(t, b)
	if sessionOf(w, claims.SID) != victim || life.get() != "connect,resume" {
		t.Fatalf("same token must resume the session (%s)", life.get())
	}
}

func TestConnectWithoutTokenIsRejected(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	ts := serveWorld(t, w)
	for _, q := range []string{"", "sid=client-minted"} {
		resp := connectStatus(t, ts, q, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%q: status %d, want 400", q, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "page token required") {
			t.Fatalf("%q: body %q", q, body)
		}
		if sessionOf(w, "client-minted") != nil {
			t.Fatal("a tokenless connect created a session")
		}
	}
	resp := connectStatus(t, ts, "sid=client-minted&r=1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tokenless reconnect: status %d, want a resync frame", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	if !strings.Contains(string(body), "resync") {
		t.Fatalf("tokenless reconnect body %q, want resync", body)
	}
}

func TestDevKeyIsUsedWhenNoKeyConfigured(t *testing.T) {
	w := NewWorld()
	tok := w.PageToken(nil)
	if _, err := w.verifyToken(tok); err != nil {
		t.Fatalf("dev-key token must verify within its process: %v", err)
	}
	other := NewWorld()
	if _, err := other.verifyToken(tok); err == nil {
		t.Fatal("two dev keys must differ")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTokenParamsDefaultToTheSSRRequestAndAreAttested(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	ts := serveWorld(t, w)
	r := httptest.NewRequest("GET", "/notes/n1?assistant=open&filter=all", nil)
	tok := w.PageToken(r, TokenParam("tenant", "acme"))
	st := openToken(t, ts, tok, nil)
	readUntilReady(t, st)
	s := sessionOf(w, claimsOf(t, w, tok).SID)
	p := s.Params()
	if p[ParamPath] != "/notes/n1" || p[ParamQuery] != "assistant=open&filter=all" || p["tenant"] != "acme" {
		t.Fatalf("attested params wrong: %v", p)
	}
	if s.Request().Params[ParamQuery] != "assistant=open&filter=all" {
		t.Fatal("RequestInfo must carry the verified params too")
	}
	p["tenant"] = "mutated"
	if s.Params()["tenant"] != "acme" {
		t.Fatal("Params() must return a copy")
	}
}

func TestTamperedParamsInvalidateTheToken(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	ts := serveWorld(t, w)
	tok := w.PageToken(httptest.NewRequest("GET", "/a?x=1", nil))
	body, sig, _ := strings.Cut(tok, ".")
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		t.Fatal(err)
	}
	forged := strings.Replace(string(raw), "x=1", "x=2", 1)
	if forged == string(raw) {
		t.Fatal("fixture: params not found in the payload")
	}
	bad := base64.RawURLEncoding.EncodeToString([]byte(forged)) + "." + sig
	if resp := connectStatus(t, ts, "t="+url.QueryEscape(bad), nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered params status %d, want 403", resp.StatusCode)
	}
}

func TestSeedCellRidesTheFirstFrameAndIsAdoptedByTheApp(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	var adopted *Cell[string]
	w.OnConnect(func(s *Session) {
		adopted = SessionCell(w, s, "assistant.open", "closed")
	})
	ts := serveWorld(t, w)
	tok := w.PageToken(nil, SeedCell("assistant.open", "open"), SeedCell("count", 3))
	st := openToken(t, ts, tok, nil)
	frames := readUntilReady(t, st)
	if count(frames, `"topic":"assistant.open","v":"open"`) != 1 {
		t.Fatalf("seed missing from the first frame: %q", frames)
	}
	if count(frames, `"topic":"count","v":3`) != 1 {
		t.Fatalf("undeclared seed must still ride the first frame: %q", frames)
	}
	if adopted == nil || adopted.Get() != "open" {
		t.Fatalf("SessionCell must adopt the seed, got %v", adopted)
	}
	adopted.Set("closed-again")
	drop(t, w, claimsOf(t, w, tok).SID, st)
	b := openToken(t, ts, tok, reconnectHeader)
	frames = readUntilReady(t, b)
	if count(frames, `"topic":"assistant.open","v":"closed-again"`) != 1 {
		t.Fatalf("reattach must not re-seed: %q", frames)
	}
}

func TestSeedCellRejectsBadInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for a malformed seed topic")
		}
	}()
	SeedCell("bad topic!", 1)
}

func TestClientReportedURLReachesTheSession(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	ts := serveWorld(t, w)
	tok := w.PageToken(httptest.NewRequest("GET", "/a", nil))
	sid := claimsOf(t, w, tok).SID
	st := openToken2(t, ts, "t="+url.QueryEscape(tok)+"&u="+url.QueryEscape("/notes/n2?filter=all"), nil)
	readUntilReady(t, st)
	s := sessionOf(w, sid)
	if got := s.URL(); got.Path != "/notes/n2" || got.RawQuery != "filter=all" {
		t.Fatalf("URL() = %v", got)
	}
	drop(t, w, sid, st)
	b := openToken2(t, ts, "t="+url.QueryEscape(tok)+"&r=1&u="+url.QueryEscape("/notes/n3"), nil)
	readUntilReady(t, b)
	if got := s.URL(); got.Path != "/notes/n3" {
		t.Fatalf("URL() after reconnect = %v", got)
	}
	if s.Params()[ParamPath] != "/a" {
		t.Fatalf("attested path must not follow the client: %v", s.Params())
	}
	for _, bad := range []string{"//evil.example/x", "https://evil.example/", `/\evil`, "javascript:1", strings.Repeat("/a", 2000)} {
		drop(t, w, sid, b)
		b = openToken2(t, ts, "t="+url.QueryEscape(tok)+"&r=1&u="+url.QueryEscape(bad), nil)
		readUntilReady(t, b)
		if got := s.URL(); got.Path != "/notes/n3" {
			t.Fatalf("hostile u %q replaced the URL: %v", bad, got)
		}
	}
}

func TestReconnectMarkerWorksWithoutLastEventID(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	w.Stream("drops.list", func() templ.Component { return htmlComp("<i>truth</i>") })
	ts := serveWorld(t, w)
	tok := w.PageToken(nil)
	a := openToken(t, ts, tok, nil)
	readUntilReady(t, a)
	drop(t, w, claimsOf(t, w, tok).SID, a)
	b := openToken2(t, ts, "t="+url.QueryEscape(tok)+"&r=1", nil)
	readUntilReady(t, b)
	f, ok := b.next(t, 2*time.Second)
	if !ok || !strings.Contains(f, "truth") {
		t.Fatalf("r=1 must trigger the stream catch-up: %q", f)
	}
	c := openToken2(t, ts, "t="+url.QueryEscape(w.PageToken(nil))+"&r=1", nil)
	if f, ok := c.next(t, 2*time.Second); !ok || !strings.Contains(f, `"kind":"resync"`) {
		t.Fatalf("unknown-session r=1 must resync: %q", f)
	}
}

func TestSessionRequestIsAllowlisted(t *testing.T) {
	w := NewWorld(WithKey(testKey), WithRequestCookies("auth"))
	ts := serveWorld(t, w)
	tok := w.PageToken(nil, TokenParam("who", "ada"))
	st := openToken(t, ts, tok, map[string]string{
		"Authorization":   "Bearer secret",
		"User-Agent":      "probe/1",
		"Accept-Language": "fr",
		"Cookie":          "auth=tok123; other=x",
		"X-Custom":        "nope",
	})
	readUntilReady(t, st)
	ri := sessionOf(w, claimsOf(t, w, tok).SID).Request()
	if ri.Header.Get("User-Agent") != "probe/1" || ri.Header.Get("Accept-Language") != "fr" {
		t.Fatalf("default headers missing: %v", ri.Header)
	}
	if ri.Header.Get("Authorization") != "" || ri.Header.Get("Cookie") != "" || ri.Header.Get("X-Custom") != "" {
		t.Fatalf("non-allowlisted headers leaked: %v", ri.Header)
	}
	if c, ok := ri.Cookie("auth"); !ok || c.Value != "tok123" {
		t.Fatalf("allowlisted cookie missing: %v", ri.Cookies)
	}
	if _, ok := ri.Cookie("other"); ok {
		t.Fatal("non-allowlisted cookie leaked")
	}
	if ri.RemoteAddr == "" || ri.Params["who"] != "ada" {
		t.Fatalf("remote addr / verified params missing: %+v", ri)
	}
	ri.Header.Set("User-Agent", "mutated")
	ri.Params["who"] = "eve"
	again := sessionOf(w, claimsOf(t, w, tok).SID).Request()
	if again.Header.Get("User-Agent") != "probe/1" || again.Params["who"] != "ada" {
		t.Fatal("Request() must return a copy")
	}
}

func TestAuthorizationIsCapturedOnlyWhenAllowlisted(t *testing.T) {
	w := NewWorld(WithKey(testKey), WithRequestHeaders("authorization"))
	ts := serveWorld(t, w)
	tok := w.PageToken(nil)
	st := openToken(t, ts, tok, map[string]string{"Authorization": "Bearer secret"})
	readUntilReady(t, st)
	if got := sessionOf(w, claimsOf(t, w, tok).SID).Request().Header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("allowlisted Authorization = %q", got)
	}
}

func TestCommandRequestCarriesTheDispatchPOST(t *testing.T) {
	w := NewWorld(WithKey(testKey), WithRequestCookies("auth"))
	var mu sync.Mutex
	var seen RequestInfo
	w.Command("who", func(s *Session, _ Args) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		seen = s.CommandRequest()
		return nil, nil
	})
	ts := serveWorld(t, w)
	tok := w.PageToken(nil, TokenParam("who", "ada"))
	st := openToken(t, ts, tok, nil)
	readUntilReady(t, st)
	sid := claimsOf(t, w, tok).SID

	req, _ := http.NewRequest("POST", ts.URL+"/live/commands", strings.NewReader(`{"sid":"`+sid+`","action":"who"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("User-Agent", "cmd/1")
	req.AddCookie(&http.Cookie{Name: "auth", Value: "c1"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("command status %d", resp.StatusCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if c, ok := seen.Cookie("auth"); !ok || c.Value != "c1" {
		t.Fatalf("command cookie missing: %+v", seen)
	}
	if seen.Header.Get("User-Agent") != "cmd/1" || seen.Header.Get("Authorization") != "" {
		t.Fatalf("command headers wrong: %v", seen.Header)
	}
	if seen.Params["who"] != "ada" {
		t.Fatalf("command params: %v", seen.Params)
	}
}

func TestTokenPayloadKeysAreSorted(t *testing.T) {
	w := NewWorld(WithKey(testKey))
	for range 20 {
		tok := w.PageToken(nil, SeedCell("z", 1), SeedCell("a", 2), SeedCell("m", 3), SeedCell("k", 4), TokenParam("q", "1"), TokenParam("b", "2"))
		body, _, _ := strings.Cut(tok, ".")
		payload, err := base64.RawURLEncoding.DecodeString(body)
		if err != nil {
			t.Fatal(err)
		}
		p := string(payload)
		last := -1
		for _, k := range []string{`"a":`, `"k":`, `"m":`, `"z":`} {
			i := strings.Index(p, k)
			if i < last {
				t.Fatalf("seeds out of order: %s", p)
			}
			last = i
		}
		if strings.Index(p, `"b":`) > strings.Index(p, `"q":`) {
			t.Fatalf("params out of order: %s", p)
		}
	}
}
