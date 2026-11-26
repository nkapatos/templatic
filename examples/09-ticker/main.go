// Command ticker shows a board of many live prices, changed many times a
// second, on every open page.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/ticker/components"
)

const distDir = ".templatic/dist"

func main() {
	addr := ":8080"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}
	symbols := envInt("TICKER_SYMBOLS", 100)
	rate := envInt("TICKER_RATE", 50)

	// Page tokens are HMAC-signed. Set TC_KEY (32+ random bytes) in
	// production; without it a per-process key is used and open pages
	// must reload after a restart.
	var opts []live.Option
	if key := os.Getenv("TC_KEY"); key != "" {
		opts = append(opts, live.WithKey([]byte(key)))
	}
	if v := os.Getenv("TICKER_WINDOW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Fatalf("ticker: TICKER_WINDOW: %v", err)
		}
		opts = append(opts, live.WithCoalesceWindow(d))
	}
	w := live.NewWorld(opts...)
	board := newBoard(w, symbols)
	go feed(board, rate, 100*time.Millisecond)

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("ticker: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", templ.Handler(components.Home(board)))
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("ticker: %d symbols, %d updates/s, listening on %s", symbols, rate, addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		log.Fatalf("ticker: %s wants a positive integer, got %q", name, v)
	}
	return n
}
