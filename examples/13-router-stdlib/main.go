// Command items serves a short list of likeable items on net/http's
// ServeMux; 13-router-chi and 13-router-echo serve the same app on chi and
// echo.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/router-stdlib/internal/items"
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
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", it.List)
	mux.Handle("GET /items/{id}", it.Show)
	mux.Handle("POST /items/{id}/like", it.LikeForm)
	// GET /live/stream and POST /live/commands.
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))
	return mux
}
