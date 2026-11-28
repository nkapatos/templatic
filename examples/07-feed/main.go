// Command feed serves chat rooms whose messages, and an activity log, are
// live streams: every open page gets each new item appended in place.
package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"

	"examples/feed/components"
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
	var opts []live.Option
	if key := os.Getenv("TC_KEY"); key != "" {
		opts = append(opts, live.WithKey([]byte(key)))
	}
	w := live.NewWorld(opts...)
	f := newFeed(w)
	go f.tick(15 * time.Second)

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("feed: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := newMux(w, f)
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("feed: listening on %s", addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}

// newMux serves the room pages and the forms' commands, with and without
// JavaScript.
func newMux(w *live.World, f *feed) *http.ServeMux {
	// page renders the room the URL names, or 404.
	page := func(rw http.ResponseWriter, r *http.Request, failed ui.Option, status int) {
		room := r.PathValue("room")
		var c templ.Component
		if slices.Contains(components.Rooms, room) {
			c = components.Room(room, f.room(room), f.activity, r.URL.Query().Get("name"), failed)
		} else {
			c, status = components.NotFound("There is no room "+room+"."), http.StatusNotFound
		}
		templ.Handler(c, templ.WithStatus(status)).ServeHTTP(rw, r)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.RedirectHandler("/rooms/"+components.Rooms[0], http.StatusSeeOther))
	mux.HandleFunc("GET /rooms/{room}", func(rw http.ResponseWriter, r *http.Request) { page(rw, r, nil, http.StatusOK) })
	// The form POSTs here; without JavaScript the browser is redirected back
	// to the room, with the name kept in the URL for the next post.
	mux.Handle("POST /rooms/{room}/messages", components.Post.Form(w,
		func(_ *http.Request, _ *live.Session, in components.PostIn) (string, error) {
			if err := f.post(in); err != nil {
				return "", err
			}
			return "/rooms/" + in.Room + "?name=" + url.QueryEscape(in.Name), nil
		},
		func(rw http.ResponseWriter, r *http.Request, err error) {
			status := live.HTTPStatus(err)
			if status == http.StatusBadRequest {
				status = http.StatusUnprocessableEntity
			}
			page(rw, r, ui.Failed(r, err), status)
		}))
	// Like and delete; without JavaScript, then back to the room. A refused
	// one (the message is gone) is a 404 page.
	back := func(do func(components.MessageIn) error) func(*http.Request, *live.Session, components.MessageIn) (string, error) {
		return func(_ *http.Request, _ *live.Session, in components.MessageIn) (string, error) {
			if err := do(in); err != nil {
				return "", err
			}
			return "/rooms/" + in.Room, nil
		}
	}
	gone := func(rw http.ResponseWriter, r *http.Request, _ error) {
		templ.Handler(components.NotFound("That message is gone."), templ.WithStatus(http.StatusNotFound)).ServeHTTP(rw, r)
	}
	mux.Handle("POST /rooms/{room}/messages/{id}/like", components.Like.Form(w, back(f.like), gone))
	mux.Handle("POST /rooms/{room}/messages/{id}/delete", components.Delete.Form(w, back(f.remove), gone))
	return mux
}
