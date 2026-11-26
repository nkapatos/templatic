// Command islandchart shows a chart library inside a lit island, fed live
// from a world cell the server appends to.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/islandchart/components"
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
	t := newTraffic(w)
	go t.sample(time.Second)

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("islandchart: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.Home(t.props())).ServeHTTP(rw, r)
	})
	mux.Handle("GET /about", templ.Handler(components.About()))
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("islandchart: listening on %s", addr)
	// App stacks the manifest and the page tokens around mux.
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}
