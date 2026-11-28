// Command pages serves static templ pages with templatic's bundled assets
// and no live state.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"

	"examples/pages/components"
)

const distDir = ".templatic/dist"

func main() {
	addr := ":8080"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("pages: %v (run `go tool templatic generate` first)", err)
	}
	// templatic.Asset resolves against this manifest.
	templatic.UseManifest(manifests.Current())

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", templ.Handler(components.Home()))
	mux.Handle("GET /about", templ.Handler(components.About()))
	mux.Handle("/", templ.Handler(components.NotFound(), templ.WithStatus(http.StatusNotFound)))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("pages: listening on %s", addr)
	// No World: App adds only the manifest layer.
	if err := http.ListenAndServe(addr, templatic.App(nil, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}
