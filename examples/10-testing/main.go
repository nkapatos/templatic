// Command testing serves one page with a shared count, a per-page tally and
// a log; its tests show how an app tests live pages, with and without a
// browser.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"

	"examples/testing/components"
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
	c := newCounter(w)
	go c.tick(context.Background(), 20*time.Second)

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("testing: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	home := func(rw http.ResponseWriter, r *http.Request, failed ui.Option, status int) {
		templ.Handler(components.Home(c.count, c.log, failed), templ.WithStatus(status)).ServeHTTP(rw, r)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) { home(rw, r, nil, http.StatusOK) })
	// The form POSTs here, with and without JavaScript; without it the page
	// has no live session, so only the count and the log change.
	mux.Handle("POST /add", c.addForm(w, func(rw http.ResponseWriter, r *http.Request, err error) {
		home(rw, r, ui.Failed(r, err), http.StatusUnprocessableEntity)
	}))
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("testing: listening on %s", addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}
