package live

import (
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func agedWorld(ttl time.Duration) (*World, func(time.Duration)) {
	var off atomic.Int64
	w := NewWorld(WithKey(testKey), WithTokenTTL(ttl))
	w.now = func() time.Time { return time.Now().Add(time.Duration(off.Load())) }
	return w, func(d time.Duration) { off.Add(int64(d)) }
}

func TestExpiredTokenReattachesALiveSession(t *testing.T) {
	w, advance := agedWorld(time.Hour)
	ts := serveWorld(t, w)
	tok := w.PageToken(nil)
	sid := claimsOf2(t, w, tok)
	a := openToken(t, ts, tok, nil)
	readUntilReady(t, a)
	s := sessionOf(w, sid)
	drop(t, w, sid, a)

	advance(2 * time.Hour)
	b := openToken(t, ts, tok, reconnectHeader)
	readUntilReady(t, b)
	if sessionOf(w, sid) != s {
		t.Fatal("expired token must reattach the same session")
	}

	b.body.Close()
	waitFor(t, "detach", func() bool { return !s.Attached() })
	s.retire()
	if resp := connectStatus(t, ts, "t="+url.QueryEscape(tok), nil); resp.StatusCode != 403 {
		t.Fatalf("fresh expired connect status %d, want 403", resp.StatusCode)
	}
}

func claimsOf2(t *testing.T, w *World, token string) string {
	t.Helper()
	c, err := w.verifyToken(token)
	if err != nil && c.SID == "" {
		t.Fatal(err)
	}
	return c.SID
}

func TestReconnectTokenFailuresResync(t *testing.T) {
	w, advance := agedWorld(time.Hour)
	ts := serveWorld(t, w)
	expired := w.PageToken(nil)
	advance(2 * time.Hour)
	other := NewWorld(WithKey([]byte("another-key-another-key-another!")))
	good := w.PageToken(nil)

	for name, q := range map[string]string{
		"expired, no session": "t=" + url.QueryEscape(expired),
		"forged":              "t=" + url.QueryEscape(other.PageToken(nil)),
		"garbage":             "t=nonsense",
		"sid mismatch":        "sid=other&t=" + url.QueryEscape(good),
	} {
		for _, hdr := range []map[string]string{reconnectHeader, nil} {
			query := q
			if hdr == nil {
				query += "&r=1"
			}
			st := openToken2(t, ts, query, hdr)
			f, ok := st.next(t, 2*time.Second)
			if !ok || !strings.Contains(f, `"kind":"resync"`) {
				t.Fatalf("%s: want a resync frame, got %q", name, f)
			}
		}
	}
	if resp := connectStatus(t, ts, "t="+url.QueryEscape(expired), nil); resp.StatusCode != 403 {
		t.Fatalf("fresh expired status %d", resp.StatusCode)
	}
	if resp := connectStatus(t, ts, "sid=other&t="+url.QueryEscape(good), nil); resp.StatusCode != 400 {
		t.Fatalf("fresh mismatch status %d", resp.StatusCode)
	}
}
