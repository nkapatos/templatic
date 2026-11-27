// Command items serves a short list of likeable items on echo; 13-router-stdlib
// serves the same app on net/http's ServeMux.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/labstack/echo/v5"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/router-echo/internal/items"
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
	e := echo.New()
	// WrapHandler copies echo's path params into r.SetPathValue, where the
	// handlers and the form decoding read {id}.
	e.GET("/", echo.WrapHandler(it.List))
	e.GET("/items/:id", echo.WrapHandler(it.Show))
	e.POST("/items/:id/like", echo.WrapHandler(it.LikeForm))
	// World.Mount takes a *http.ServeMux, so the world's two handlers are
	// mounted one by one, at the paths Boot's Live base names.
	e.GET("/live/stream", echo.WrapHandler(w.SSE()))
	e.POST("/live/commands", echo.WrapHandler(w.Commands()))
	e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", http.FileServer(http.Dir("public")))))
	e.GET("/_templatic/*", echo.WrapHandler(http.StripPrefix("/_templatic/", templatic.Assets(distDir))))
	return e
}
