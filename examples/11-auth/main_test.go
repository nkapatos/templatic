package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/auth/components"
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

// status returns the HTTP status err answers with, 204 for none.
func status(err error) int {
	var se *live.StatusError
	switch {
	case err == nil:
		return http.StatusNoContent
	case errors.As(err, &se):
		return se.Code
	default:
		return http.StatusInternalServerError
	}
}

func TestCommandAuthorization(t *testing.T) {
	w := live.NewWorld(live.WithRequestCookies(cookieName))
	l := newLogins()
	newApp(w, nil, l)
	aliceCookie, bobCookie := l.login("alice", "alice"), l.login("bob", "bob")

	alice := page(t, w, "alice", aliceCookie)
	bob := page(t, w, "bob", bobCookie)
	note := components.NoticeIn{Text: "hi"}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"admin posts", first(livetest.Call(alice, components.PostNotice, note)), http.StatusNoContent},
		{"viewer posts", first(livetest.Call(bob, components.PostNotice, note)), http.StatusForbidden},
		{"viewer waves", first(livetest.Call(bob, components.Wave, components.WaveIn{})), http.StatusNoContent},
		// bob's cookie on alice's page: the token says alice, the cookie bob.
		{"viewer on the admin's page", first(livetest.Call(page(t, w, "alice", bobCookie), components.PostNotice, note)), http.StatusUnauthorized},
		{"no cookie", first(livetest.Call(page(t, w, "alice", ""), components.Wave, components.WaveIn{})), http.StatusUnauthorized},
	} {
		if got := status(tc.err); got != tc.want {
			t.Errorf("%s: status %d, want %d (%v)", tc.name, got, tc.want, tc.err)
		}
	}

	l.logout(bobCookie)
	if got := status(first(livetest.Call(bob, components.Wave, components.WaveIn{}))); got != http.StatusUnauthorized {
		t.Errorf("after logout: status %d, want 401", got)
	}
}

// first drops a command's result and keeps its error.
func first(_ any, err error) error { return err }
