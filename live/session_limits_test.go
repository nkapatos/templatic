package live

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// untilResync reads frames until a resync and returns it.
func untilResync(t *testing.T, st *stream) string {
	t.Helper()
	for {
		f, ok := st.next(t, 2*time.Second)
		if !ok || f == "" {
			t.Fatal("no resync arrived")
		}
		if strings.Contains(f, `"kind":"resync"`) {
			return f
		}
	}
}

func TestEndSessionsResyncsMatchingPagesOnly(t *testing.T) {
	w := NewWorld(WithResumeGrace(time.Minute))
	ts := serveWorld(t, w)
	alice := w.PageToken(nil, TokenParam("user", "alice"))
	bob := w.PageToken(nil, TokenParam("user", "bob"))
	a := openToken(t, ts, alice, nil)
	b := openToken(t, ts, bob, nil)
	readUntilReady(t, a)
	readUntilReady(t, b)

	n := w.EndSessions(func(s *Session) bool { return s.Params()["user"] == "alice" })
	if n != 1 {
		t.Fatalf("ended %d sessions, want 1", n)
	}
	if f := untilResync(t, a); !strings.Contains(f, `"reason":"revoked"`) {
		t.Fatalf("got %q", f)
	}
	if sessionOf(w, claimsOf(t, w, alice).SID) != nil {
		t.Fatal("alice's session is still held")
	}
	if sessionOf(w, claimsOf(t, w, bob).SID) == nil {
		t.Fatal("bob's session ended too")
	}
}

func TestAdmitRefusesConnectAndEndsReattachingSession(t *testing.T) {
	allowed := map[string]bool{"alice": true}
	w := NewWorld(WithResumeGrace(time.Minute), WithAdmit(func(_ *http.Request, p map[string]string) bool {
		return allowed[p["user"]]
	}))
	ts := serveWorld(t, w)
	if resp := connectStatus(t, ts, "t="+url.QueryEscape(w.PageToken(nil, TokenParam("user", "mallory"))), nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("refused new connect: status %d, want 403", resp.StatusCode)
	}

	tok := w.PageToken(nil, TokenParam("user", "alice"))
	sid := claimsOf(t, w, tok).SID
	a := openToken(t, ts, tok, nil)
	readUntilReady(t, a)
	drop(t, w, sid, a)

	delete(allowed, "alice") // logged out
	resp := connectStatus(t, ts, "r=1&t="+url.QueryEscape(tok), nil)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"kind":"resync","reason":"refused"`) {
		t.Fatalf("refused reattach got %q", body)
	}
	if sessionOf(w, sid) != nil {
		t.Fatal("a refused reattach must end the session")
	}
}

func TestMaxSessionAgeEndsAConnectedSession(t *testing.T) {
	w := NewWorld(WithMaxSessionAge(100 * time.Millisecond))
	ts := serveWorld(t, w)
	r := openStream(t, ts, "s1")
	defer r.Close()
	r.discardUntilReady(t)
	if f := untilResync(t, r); !strings.Contains(f, `"reason":"max-age"`) {
		t.Fatalf("got %q", f)
	}
	if sessionOf(w, "s1") != nil {
		t.Fatal("an aged session is still held")
	}
}

func TestEachAttachRotatesTheToken(t *testing.T) {
	w := NewWorld(WithResumeGrace(time.Minute))
	ts := serveWorld(t, w)
	t0 := w.PageToken(nil)
	sid := claimsOf(t, w, t0).SID
	attach := func(tok string, headers map[string]string) string {
		t.Helper()
		st := openToken(t, ts, tok, headers)
		readUntilReady(t, st)
		next := st.latestToken()
		drop(t, w, sid, st)
		return next
	}

	t1 := attach(t0, nil)
	if t1 == "" || t1 == t0 {
		t.Fatalf("first attach sent no fresh token: %q", t1)
	}
	t2 := attach(t0, reconnectHeader) // the page lost t1's frame: the token before the newest still reattaches
	if resp := connectStatus(t, ts, "r=1&t="+url.QueryEscape(t0), nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a token two attaches old: status %d, want 403", resp.StatusCode)
	}
	if t3 := attach(t2, reconnectHeader); t3 == "" {
		t.Fatal("the newest token must reattach")
	}
}
