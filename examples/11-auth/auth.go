package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"net/http"
	"sync"

	"github.com/nkapatos/templatic/live"

	"examples/auth/components"
)

// cookieName is the login cookie; the world allowlists it so commands see it.
const cookieName = "auth"

// userParam is the page-token param that attests the page's user.
const userParam = "user"

// account is a demo user: a plain password, never do this in a real app.
type account struct {
	password string
	admin    bool
}

// logins holds the demo users and the signed-in browsers (cookie value to name).
type logins struct {
	users map[string]account

	mu     sync.Mutex
	active map[string]string
}

func newLogins() *logins {
	return &logins{
		users: map[string]account{
			"alice": {password: "alice", admin: true},
			"bob":   {password: "bob"},
		},
		active: map[string]string{},
	}
}

// login checks the password and returns a new cookie value, or "".
func (l *logins) login(name, password string) string {
	a, ok := l.users[name]
	if !ok || subtle.ConstantTimeCompare([]byte(a.password), []byte(password)) != 1 {
		return ""
	}
	id := rand.Text()
	l.mu.Lock()
	l.active[id] = name
	l.mu.Unlock()
	return id
}

// logout forgets the cookie value.
func (l *logins) logout(id string) {
	l.mu.Lock()
	delete(l.active, id)
	l.mu.Unlock()
}

// signOut forgets the cookie value and ends the live pages that browser has open.
func (l *logins) signOut(w *live.World, id string) {
	l.logout(id)
	w.EndSessions(func(s *live.Session) bool {
		c, ok := s.Request().Cookie(cookieName)
		return ok && c.Value == id
	})
}

// admits reports whether r's browser is signed in as the user of the page whose params these are.
func (l *logins) admits(r *http.Request, params map[string]string) bool {
	u, ok := l.fromRequest(r)
	return ok && u.Name == params[userParam]
}

// lookup returns the user a cookie value is signed in as.
func (l *logins) lookup(id string) (components.User, bool) {
	l.mu.Lock()
	name, ok := l.active[id]
	l.mu.Unlock()
	if !ok {
		return components.User{}, false
	}
	return components.User{Name: name, Admin: l.users[name].admin}, true
}

// fromRequest returns the user r's login cookie names.
func (l *logins) fromRequest(r *http.Request) (components.User, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return components.User{}, false
	}
	return l.lookup(c.Value)
}

// formUser returns the user a form runs as: the login cookie's (signedIn
// put it in r), refused with 401 when the live page s belongs to someone else.
func formUser(r *http.Request, s *live.Session) (components.User, error) {
	u := userOf(r)
	if s != nil && s.Params()[userParam] != u.Name {
		return components.User{}, live.Status(http.StatusUnauthorized, "log in again")
	}
	return u, nil
}

// requireAdmin refuses a non-admin with 403.
func requireAdmin(u components.User) error {
	if !u.Admin {
		return live.Status(http.StatusForbidden, "admins only")
	}
	return nil
}

type userKey struct{}

// signedIn serves next with the request's user in the context; without a
// login it redirects a GET to /login and refuses anything else with 401.
func (l *logins) signedIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		u, ok := l.fromRequest(r)
		if !ok {
			if r.Method == http.MethodGet {
				http.Redirect(rw, r, "/login", http.StatusSeeOther)
				return
			}
			http.Error(rw, "log in again", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}

// userOf returns the user signedIn put in r's context.
func userOf(r *http.Request) components.User {
	u, _ := r.Context().Value(userKey{}).(components.User)
	return u
}
