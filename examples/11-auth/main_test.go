package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/live"
)

// tokenRE finds the page token in Boot's config.
var tokenRE = regexp.MustCompile(`"token":"([^"]+)"`)

// signIn logs name in on a fresh client, opens the board's live stream and
// returns the client and the page's session id.
func signIn(t *testing.T, srv *httptest.Server, name string) (*http.Client, string) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	res, err := c.PostForm(srv.URL+"/login", url.Values{"name": {name}, "password": {name}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	m := tokenRE.FindSubmatch(page)
	if m == nil {
		t.Fatalf("%s: no page token on %s", name, res.Request.URL)
	}
	// The stream creates the session; reading its first line means it is attached.
	stream, err := c.Get(srv.URL + "/live/stream?t=" + url.QueryEscape(string(m[1])))
	if err != nil || stream.StatusCode != http.StatusOK {
		t.Fatalf("%s: stream: %v %v", name, err, stream.Status)
	}
	if _, err := bufio.NewReader(stream.Body).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stream.Body.Close() })
	// The sid is the token payload's "s", as the client runtime reads it.
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(string(m[1]), ".")[0])
	var claims struct {
		SID string `json:"s"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	return c, claims.SID
}

// command raises action on sid through the HTTP command endpoint.
func command(t *testing.T, c *http.Client, srv *httptest.Server, sid, action, args string) int {
	t.Helper()
	res, err := c.Post(srv.URL+"/live/commands", "application/json",
		strings.NewReader(`{"sid":"`+sid+`","action":"`+action+`","args":`+args+`}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestCommandAuthorization(t *testing.T) {
	srv := httptest.NewServer(newApp(live.NewWorld(live.WithRequestCookies(cookieName)), nil, newLogins()))
	t.Cleanup(srv.Close) // runs after the streams' cleanups close them
	alice, aliceSID := signIn(t, srv, "alice")
	bob, bobSID := signIn(t, srv, "bob")

	for _, tc := range []struct {
		name   string
		c      *http.Client
		sid    string
		action string
		want   int
	}{
		{"admin posts", alice, aliceSID, "notice.post", http.StatusNoContent},
		{"viewer posts", bob, bobSID, "notice.post", http.StatusForbidden},
		{"viewer waves", bob, bobSID, "wave", http.StatusNoContent},
		{"viewer on the admin's page", bob, aliceSID, "notice.post", http.StatusUnauthorized},
		{"no cookie", http.DefaultClient, aliceSID, "wave", http.StatusUnauthorized},
	} {
		if got := command(t, tc.c, srv, tc.sid, tc.action, `{"text":"hi"}`); got != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.want)
		}
	}

	if _, err := bob.Post(srv.URL+"/logout", "", nil); err != nil {
		t.Fatal(err)
	}
	if got := command(t, bob, srv, bobSID, "wave", `{}`); got != http.StatusUnauthorized {
		t.Errorf("after logout: status %d, want 401", got)
	}
}
