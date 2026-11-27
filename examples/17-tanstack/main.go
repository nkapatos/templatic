// Command tanstack runs TanStack's headless libraries in lit islands: a
// virtual list fed by a world cell, a client-side table and a lit-form form
// whose server command still decides.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"

	"examples/tanstack/components"
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
	feed := newLogFeed(w)
	go feed.run()
	orders := newOrders(w)
	go changeOrders(orders, 3*time.Second)
	users := newSignups(w, "ada", "grace")
	signup := components.SignupProps{Plans: components.Plans}

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("tanstack: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.LogPage(feed.props(), feed.rate)).ServeHTTP(rw, r)
	})
	mux.HandleFunc("GET /table", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.TablePage(components.TableProps{Orders: orders.Get()})).ServeHTTP(rw, r)
	})
	mux.Handle("GET /signup", templ.Handler(components.SignupPage(signup, nil, users.taken)))
	// Without JavaScript the plain form POSTs here: success redirects back,
	// refused input renders the page again with 422.
	mux.Handle("POST /signup", components.Register.FormPOST(w,
		func(r *http.Request, in components.SignupIn) (string, error) {
			return "/signup", users.register(in)
		},
		func(rw http.ResponseWriter, r *http.Request, err error) {
			page := components.SignupPage(signup, ui.Failed(r, err), users.taken)
			templ.Handler(page, templ.WithStatus(http.StatusUnprocessableEntity)).ServeHTTP(rw, r)
		}))
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("tanstack: listening on %s", addr)
	// App stacks the manifest and the page tokens around mux.
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}
