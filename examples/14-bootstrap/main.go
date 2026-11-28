// Command bootstrap brings Bootstrap 5 from npm into a templatic app: its
// CSS and JS in the app bundle, data-API widgets on live markup, tooltips
// and popovers through hooks.
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

	"examples/bootstrap/components"
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
	go b.measure(5 * time.Second)

	// The generated manifest. TC_MODE=dev follows `templatic dev`
	// rebuilds and reloads open pages; otherwise it is read once.
	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("bootstrap: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	report := func(rw http.ResponseWriter, r *http.Request, failed ui.Option, status int) {
		c := components.ReportPage(b.all(), r.URL.Query().Get("service"), failed)
		templ.Handler(c, templ.WithStatus(status)).ServeHTTP(rw, r)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.Home(b.region, b.all())).ServeHTTP(rw, r)
	})
	mux.HandleFunc("GET /report", func(rw http.ResponseWriter, r *http.Request) {
		report(rw, r, nil, http.StatusOK)
	})
	// Without JS the forms POST here and the browser follows the redirect
	// back to the board. The status buttons cannot send invalid input, so
	// a refusal there is a plain 400.
	mux.Handle("POST /set", components.Set.Form(w,
		func(_ *http.Request, _ *live.Session, in components.SetIn) (string, error) { return "/", b.set(in) },
		nil))
	mux.Handle("POST /report", components.Report.Form(w,
		func(_ *http.Request, _ *live.Session, in components.ReportIn) (string, error) {
			return "/", b.report(in)
		},
		func(rw http.ResponseWriter, r *http.Request, err error) {
			report(rw, r, ui.Failed(r, err), http.StatusUnprocessableEntity)
		}))
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		templ.Handler(components.NotFound(), templ.WithStatus(http.StatusNotFound)).ServeHTTP(rw, r)
	})
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("bootstrap: listening on %s", addr)
	// App stacks the manifest and the page tokens around mux.
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}
