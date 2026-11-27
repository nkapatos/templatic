// Command table is a server-driven data table: paging, search and sort live
// in the URL, and every open page refreshes with its own view when the data
// changes.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"

	"examples/table/components"
)

const distDir = ".templatic/dist"

// viewOf is the View each session's table last showed.
var viewOf = live.DeclareSessionValue("table.view", func() components.View { return components.View{}.Clean() })

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
	st := newStore(300, time.Now())
	t := newTable(w, st)
	go func() {
		for now := range time.Tick(3 * time.Second) {
			if err := t.update(now); err != nil {
				log.Printf("table: refresh: %v", err)
			}
		}
	}()

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("table: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	// Without JS (and on every reload or deep link) the query is the state.
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		res := st.Query(components.ParseView(r.URL.Query()))
		templ.Handler(components.Page(t.rows, t.last, res)).ServeHTTP(rw, r)
	})
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("table: listening on %s", addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}

// table wires the store to a world: the Navigate handler, the summary
// cells and the per-session refresh.
type table struct {
	w    *live.World
	st   *store
	rows *live.Cell[int]
	last *live.Cell[string]
}

func newTable(w *live.World, st *store) *table {
	t := &table{
		w:    w,
		st:   st,
		rows: live.NewCell(w, "orders.rows", st.Len()),
		last: live.NewCell(w, "orders.last", "none yet"),
	}
	// A new session starts at the view its page was loaded with.
	w.OnConnect(func(s *live.Session) {
		u := s.URL()
		viewOf.Set(s, components.ParseView(u.Query()))
	})
	components.Navigate.Handle(w, t.navigate)
	return t
}

// navigate swaps the table for this session only, remembers the view for
// later refreshes and declares the URL the session is now at.
func (t *table) navigate(s *live.Session, in components.View) (any, error) {
	res := t.st.Query(in.Clean())
	viewOf.Set(s, res.View)
	if err := components.TableRegion.Swap(s, res); err != nil {
		return nil, err
	}
	return live.At(res.View.URL()), nil
}

// update changes the data, then re-renders every session's table from that
// session's own view.
func (t *table) update(now time.Time) error {
	t.st.tick(now)
	t.rows.Set(t.st.Len())
	t.last.Set(now.Format("15:04:05"))
	return components.TableRegion.Refresh(t.w, func(s *live.Session) (components.Result, bool) {
		return t.st.Query(viewOf.Get(s)), true
	})
}
