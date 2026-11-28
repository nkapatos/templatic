// Command urlstate keeps each viewer's place in an inbox (tab, search, open
// item) in the URL, swapped in place with JavaScript and loaded as a plain
// page without it.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/urlstate/components"
)

const distDir = ".templatic/dist"

func main() {
	addr := ":8080"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}

	var opts []live.Option
	if key := os.Getenv("TC_KEY"); key != "" {
		opts = append(opts, live.WithKey([]byte(key)))
	}
	w := live.NewWorld(opts...)

	components.Navigate.Handle(w, navigate)

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("urlstate: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	// Without JS (and on every reload or deep link) the query is the state.
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		v := components.ParseView(r.URL.Query())
		templ.Handler(components.Page(v)).ServeHTTP(rw, r)
	})
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("urlstate: listening on %s", addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}

// navigate swaps the inbox for this session only and declares the URL the
// session is now at; the runtime pushes it onto the history. Refining a
// search replaces the entry, so typing is one step back, not one per pause.
func navigate(s *live.Session, in components.View) (any, error) {
	v := in.Clean()
	u := s.URL()
	was := components.ParseView(u.Query()).Clean()
	if err := components.InboxRegion.Swap(s, v); err != nil {
		return nil, err
	}
	if was.Q != "" && v.Q != "" && was.WithQ(v.Q) == v {
		return live.Replace(v.URL()), nil
	}
	return live.At(v.URL()), nil
}
