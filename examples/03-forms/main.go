// Command forms serves a reading list whose forms work with and without
// JavaScript: one command, one input type, two transports.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"

	"examples/forms/components"
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
	books := newStore(
		components.Book{Title: "The Name of the Rose", Pages: 512, Shelf: "done", Favorite: true, Tags: []string{"fiction", "history"}},
		components.Book{Title: "A Short History of Nearly Everything", Pages: 544, Shelf: "reading", Tags: []string{"science"}},
	)
	list, save := wire(w, books)

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("forms: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	// page renders the page a URL names: the list, a book's form, or 404.
	page := func(rw http.ResponseWriter, r *http.Request, failed ui.Option, status int) {
		var c templ.Component
		if id := r.PathValue("id"); id == "" {
			c = components.Home(list, failed)
		} else if b, ok := books.get(id); ok {
			c = components.Edit(list, b, failed)
		} else {
			c, status = components.NotFound("There is no book "+id+"."), http.StatusNotFound
		}
		templ.Handler(c, templ.WithStatus(status)).ServeHTTP(rw, r)
	}
	// Without JavaScript the forms POST here: success redirects (303) to the
	// page the book is on, refused input renders that page again with 422.
	saveForm := components.Save.FormPOST(w,
		func(r *http.Request, in components.BookIn) (string, error) {
			b, err := save(in)
			if err != nil {
				return "", err
			}
			if r.PathValue("id") == "" {
				return "/", nil
			}
			return "/books/" + b.ID, nil
		},
		func(rw http.ResponseWriter, r *http.Request, err error) {
			page(rw, r, ui.Failed(r, err), http.StatusUnprocessableEntity)
		})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) { page(rw, r, nil, http.StatusOK) })
	mux.HandleFunc("GET /books/{id}", func(rw http.ResponseWriter, r *http.Request) { page(rw, r, nil, http.StatusOK) })
	mux.Handle("POST /books", saveForm)
	mux.Handle("POST /books/{id}", saveForm)
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("forms: listening on %s", addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}

// wire declares the books region and registers Save's JS handler on w. It
// returns the region and the save step both transports run.
func wire(w *live.World, books *store) (*live.Region, func(components.BookIn) (components.Book, error)) {
	list := w.Region("books", func() templ.Component { return components.BookList(books.all()) })
	save := func(in components.BookIn) (components.Book, error) {
		b, err := books.save(in)
		if err != nil {
			return b, err
		}
		// Every open page showing the list gets the new markup.
		if err := w.Redraw(list.Name()); err != nil {
			log.Printf("forms: redraw: %v", err)
		}
		return b, nil
	}
	components.Save.Handle(w, func(_ *live.Session, in components.BookIn) (any, error) {
		_, err := save(in)
		return nil, err
	})
	return list, save
}
