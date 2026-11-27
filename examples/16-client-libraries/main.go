// Command clientlibs uses front-end libraries from npm beside the server's
// live state: SortableJS in a hook, Leaflet lazy in an island, Intl in a hook.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/clientlibs/components"
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
	b := newBoard(w)
	go b.tick(2 * time.Second)

	components.Move.Handle(w, func(_ *live.Session, in components.MoveIn) (any, error) {
		return nil, b.move(in)
	})

	// The generated manifest. TC_MODE=dev follows `templatic dev`
	// rebuilds and reloads open pages; otherwise it is read once.
	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("clientlibs: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.Home(b.queue, b.stats)).ServeHTTP(rw, r)
	})
	mux.Handle("GET /map", templ.Handler(components.MapPage(places)))
	// Without JS the arrows POST here and the browser follows the redirect
	// back. They cannot send a bad input, so a refusal is a plain 400.
	mux.Handle("POST /move", components.Move.FormPOST(w,
		func(_ *http.Request, in components.MoveIn) (string, error) { return "/", b.move(in) },
		nil))
	w.Mount(mux, "/live")
	// The app's own static files, served by the app.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	// templatic's generated bundle: hashed files are cached for a year,
	// boot.js and tc.json revalidate.
	//
	// For a single binary, embed the bundle instead (`all:` is needed
	// because embed skips dot directories):
	//
	//	//go:embed all:.templatic/dist
	//	var dist embed.FS
	//	sub, _ := fs.Sub(dist, ".templatic/dist")
	//	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", http.FileServerFS(sub)))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("clientlibs: listening on %s", addr)
	// App stacks the manifest and the page tokens around mux.
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}

// places are the map's markers.
var places = components.MapProps{Places: []components.Place{
	{Name: "Athens", Lat: 37.9838, Lng: 23.7275},
	{Name: "Berlin", Lat: 52.52, Lng: 13.405},
	{Name: "Lisbon", Lat: 38.7223, Lng: -9.1393},
	{Name: "Helsinki", Lat: 60.1699, Lng: 24.9384},
}}
