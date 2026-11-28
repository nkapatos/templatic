// Command auth signs users in with a cookie, ties each page to its user
// through the page token, and runs a command only for admins.
package main

import (
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"

	"examples/auth/components"
)

const distDir = ".templatic/dist"

func main() {
	addr := ":8080"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}

	// Page tokens are HMAC-signed. Set TC_KEY (32+ random bytes) in
	// production; without it a per-process key is used and open pages
	// must reload after a restart.
	l := newLogins()
	// A page's stream connects only while its browser is signed in as the page's user.
	opts := []live.Option{live.WithRequestCookies(cookieName), live.WithAdmit(l.admits)}
	if key := os.Getenv("TC_KEY"); key != "" {
		opts = append(opts, live.WithKey([]byte(key)))
	}
	w := live.NewWorld(opts...)

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("auth: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	log.Printf("auth: listening on %s", addr)
	if err := http.ListenAndServe(addr, newApp(w, manifests, l)); err != nil {
		log.Fatal(err)
	}
}

// newApp registers the commands on w and returns the app's handler.
func newApp(w *live.World, manifests *templatic.ManifestSource, l *logins) http.Handler {
	b := newBoard(w)

	page := func(rw http.ResponseWriter, r *http.Request, failed ui.Option, status int) {
		u := userOf(r)
		templatic.SeedPage(r, live.TokenParam(userParam, u.Name))
		templ.Handler(components.Board(u, b.notice, b.log, failed), templ.WithStatus(status)).ServeHTTP(rw, r)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", l.signedIn(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		page(rw, r, nil, http.StatusOK)
	})))
	mux.HandleFunc("GET /login", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.Login("")).ServeHTTP(rw, r)
	})
	mux.HandleFunc("POST /login", func(rw http.ResponseWriter, r *http.Request) {
		id := l.login(r.PostFormValue("name"), r.PostFormValue("password"))
		if id == "" {
			templ.Handler(components.Login("Wrong name or password."), templ.WithStatus(http.StatusUnauthorized)).ServeHTTP(rw, r)
			return
		}
		http.SetCookie(rw, &http.Cookie{Name: cookieName, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(rw, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /logout", func(rw http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(cookieName); err == nil {
			l.signOut(w, c.Value)
		}
		http.SetCookie(rw, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(rw, r, "/login", http.StatusSeeOther)
	})
	// The forms POST here, with and without JavaScript; signedIn refuses a
	// browser without a login (401).
	refused := func(rw http.ResponseWriter, r *http.Request, err error) {
		status := live.HTTPStatus(err)
		if status == http.StatusBadRequest {
			status = http.StatusUnprocessableEntity
		}
		page(rw, r, ui.Failed(r, err), status)
	}
	mux.Handle("POST /notice", l.signedIn(components.PostNotice.Form(w,
		func(r *http.Request, s *live.Session, in components.NoticeIn) (string, error) {
			u, err := formUser(r, s)
			if err != nil {
				return "", err
			}
			if err := requireAdmin(u); err != nil {
				return "", err
			}
			b.post(u, in.Text)
			return "/", nil
		}, refused)))
	mux.Handle("POST /wave", l.signedIn(components.Wave.Form(w,
		func(r *http.Request, s *live.Session, _ components.WaveIn) (string, error) {
			u, err := formUser(r, s)
			if err != nil {
				return "", err
			}
			b.wave(u)
			return "/", nil
		}, refused)))
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	// The forms have no CSRF token of their own: refuse
	// cross-origin POSTs for the whole app.
	return http.NewCrossOriginProtection().Handler(templatic.App(w, manifests, mux))
}

// board is the shared state every signed-in page shows.
type board struct {
	w      *live.World
	notice *live.Cell[string]
	log    *live.Region

	mu      sync.Mutex
	entries []components.Entry
}

func newBoard(w *live.World) *board {
	b := &board{w: w, notice: live.NewCell(w, "notice", "Welcome. No notice yet.")}
	b.log = w.Region("activity", func() templ.Component {
		b.mu.Lock()
		defer b.mu.Unlock()
		return components.Activity(slices.Clone(b.entries))
	})
	return b
}

func (b *board) post(u components.User, text string) {
	text = strings.TrimSpace(text)
	b.notice.Set(text)
	b.add(components.Entry{Who: u.Name, What: "posted “" + text + "”"})
}

func (b *board) wave(u components.User) {
	b.add(components.Entry{Who: u.Name, What: "waved"})
}

// add puts e on top of the log, keeping the last ten, and redraws it.
func (b *board) add(e components.Entry) {
	b.mu.Lock()
	b.entries = append([]components.Entry{e}, b.entries[:min(len(b.entries), 9)]...)
	b.mu.Unlock()
	if err := b.w.Redraw(b.log.Name()); err != nil {
		log.Printf("auth: redraw: %v", err)
	}
}
