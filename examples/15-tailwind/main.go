// Command tailwind styles a live page with Tailwind CSS, built by the
// app's own npm script beside templatic.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"

	"examples/tailwind/components"
)

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
	count := live.NewCell(w, "count", 0)
	level := w.Region("level", func() templ.Component { return components.Level(count.Get()) })

	add := func(n int) {
		count.Update(func(v int) int { return v + n })
		if err := w.Redraw(level.Name()); err != nil {
			log.Printf("tailwind: redraw: %v", err)
		}
	}
	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("tailwind: %v (run `npm run generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", templ.Handler(components.Home(count, level, nil)))
	mux.Handle("POST /add", components.Add.Form(w,
		func(_ *http.Request, _ *live.Session, in components.AddIn) (string, error) {
			add(in.N)
			return "/", nil
		},
		func(rw http.ResponseWriter, r *http.Request, err error) {
			templ.Handler(components.Home(count, level, ui.Failed(r, err)), templ.WithStatus(http.StatusUnprocessableEntity)).ServeHTTP(rw, r)
		}))
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("tailwind: listening on %s", addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}
