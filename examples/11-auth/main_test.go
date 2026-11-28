package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"
)

// page connects a page for user, with cookie as its login cookie ("" for none).
func page(t *testing.T, w *live.World, user, cookie string) *livetest.Session {
	t.Helper()
	r := httptest.NewRequest("GET", "/", nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
	}
	return livetest.New(t, w, livetest.Param(userParam, user), livetest.Request(r))
}

func TestFormAuthorization(t *testing.T) {
	w := live.NewWorld(live.WithRequestCookies(cookieName))
	l := newLogins()
	app := newApp(w, nil, l)
	aliceCookie, bobCookie := l.login("alice", "alice"), l.login("bob", "bob")

	alice := page(t, w, "alice", aliceCookie)
	bob := page(t, w, "bob", bobCookie)
	note := url.Values{"text": {"hi"}}
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"admin posts", alice.PostForm(app, "/notice", note).Code, http.StatusNoContent},
		{"viewer posts", bob.PostForm(app, "/notice", note).Code, http.StatusForbidden},
		{"viewer waves", bob.PostForm(app, "/wave", nil).Code, http.StatusNoContent},
		// bob's cookie on alice's page: the token says alice, the cookie bob.
		{"viewer on the admin's page", page(t, w, "alice", bobCookie).PostForm(app, "/notice", note).Code, http.StatusUnauthorized},
		{"no cookie", page(t, w, "alice", "").PostForm(app, "/wave", nil).Code, http.StatusUnauthorized},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, tc.got, tc.want)
		}
	}

	l.logout(bobCookie)
	if got := bob.PostForm(app, "/wave", nil).Code; got != http.StatusUnauthorized {
		t.Errorf("after logout: status %d, want 401", got)
	}
}

func TestSignOutEndsThatBrowsersPages(t *testing.T) {
	w := live.NewWorld(live.WithRequestCookies(cookieName))
	l := newLogins()
	newApp(w, nil, l)
	aliceCookie, bobCookie := l.login("alice", "alice"), l.login("bob", "bob")
	bobTab1, bobTab2 := page(t, w, "bob", bobCookie), page(t, w, "bob", bobCookie)
	alice := page(t, w, "alice", aliceCookie)

	l.signOut(w, bobCookie)
	for name, p := range map[string]*livetest.Session{"bob's first tab": bobTab1, "bob's second tab": bobTab2} {
		if p.Drop(); p.Resume() {
			t.Errorf("%s still reattaches after sign-out", name)
		}
	}
	if alice.Drop(); !alice.Resume() {
		t.Error("alice's page ended with bob's")
	}
}

func TestAdmitsOnlyTheSignedInUser(t *testing.T) {
	l := newLogins()
	bobCookie := l.login("bob", "bob")
	r := httptest.NewRequest("GET", "/live/stream", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: bobCookie})
	if !l.admits(r, map[string]string{userParam: "bob"}) {
		t.Error("bob's browser refused on bob's page")
	}
	if l.admits(r, map[string]string{userParam: "alice"}) {
		t.Error("bob's browser admitted on alice's page")
	}
	l.logout(bobCookie)
	if l.admits(r, map[string]string{userParam: "bob"}) {
		t.Error("a signed-out browser admitted")
	}
}
