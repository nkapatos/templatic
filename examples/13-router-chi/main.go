// Command items serves a short list of likeable items on chi; 13-router-stdlib
// serves the same app on net/http's ServeMux.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/router-chi/internal/items"
)

// distDir is where `templatic generate` writes the bundle and tc.json.
const distDir = ".templatic/dist"

func main() {
	addr := ":8080"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}

	// Page tokens are HMAC-signed. Set TC_KEY (32+ random bytes) in
	// production; without it a per-process development key is used and
	// pages must reload after every restart.
	var opts []live.Option
	if key := os.Getenv("TC_KEY"); key != "" {
		opts = append(opts, live.WithKey([]byte(key)))
	}
	w := live.NewWorld(opts...)

	// TC_MODE=dev follows `templatic dev` rebuilds and reloads open pages;
	// otherwise the manifest is read once.
	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("items: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	log.Printf("items: listening on %s", addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, routes(w, items.Register(w)))); err != nil {
		log.Fatal(err)
	}
}

// routes is everything that differs between the three router examples.
func routes(w *live.World, it *items.Feature) http.Handler {
	r := chi.NewRouter()
	r.Method("GET", "/", it.List)
	r.Method("GET", "/items/{id}", it.Show)
	r.Method("POST", "/items/{id}/like", it.LikeForm)
	// World.Mount takes a *http.ServeMux, so the world's two handlers are
	// mounted one by one, at the paths Boot's Live base names.
	r.Method("GET", "/live/stream", w.SSE())
	r.Method("POST", "/live/commands", w.Commands())
	r.Method("GET", "/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	r.Method("GET", "/_templatic/*", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))
	return r
}
