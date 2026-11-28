// Command hooks shows client-only behavior with hooks: a menu, a theme
// toggle and a modal dialog that run in the browser without a request to
// the server, each with a usable no-JS fallback.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/hooks/components"
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

	// The one server round-trip on the page: the next tip is swapped into
	// this page's card, and the URL follows it.
	components.ShowTip.Handle(w, func(s *live.Session, in components.TipIn) (any, error) {
		if err := components.TipCardRegion.Swap(s, in.N); err != nil {
			return nil, err
		}
		return live.At(components.TipURL(in.N)), nil
	})

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("hooks: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		tip := components.TipIndex(r.URL.Query().Get("tip"))
		templ.Handler(components.Home(components.PageFor(r), tip)).ServeHTTP(rw, r)
	})
	mux.HandleFunc("GET /help", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.Help(components.PageFor(r))).ServeHTTP(rw, r)
	})
	// The theme toggle without JS; with JS the theme hook writes the same
	// cookie itself.
	mux.HandleFunc("POST /theme", components.SetTheme)
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.NotFound(components.PageFor(r)), templ.WithStatus(http.StatusNotFound)).ServeHTTP(rw, r)
	})
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("hooks: listening on %s", addr)
	// App stacks the manifest and the page tokens around mux.
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}
