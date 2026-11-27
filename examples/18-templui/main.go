// Command templui is a shared task list built from templUI components in
// a templatic app.
package main

import (
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"

	"examples/templui/components"
)

const distDir = ".templatic/dist"

// store holds the tasks and redraws the tasks region on every change.
type store struct {
	w      *live.World
	region *live.Region

	mu    sync.Mutex
	next  int
	tasks []components.Task
}

func (s *store) all() []components.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.tasks)
}

func (s *store) add(title string) {
	s.mu.Lock()
	s.next++
	s.tasks = append(s.tasks, components.Task{ID: s.next, Title: strings.TrimSpace(title)})
	s.mu.Unlock()
	s.redraw()
}

func (s *store) toggle(id int) error {
	s.mu.Lock()
	i := slices.IndexFunc(s.tasks, func(t components.Task) bool { return t.ID == id })
	if i >= 0 {
		s.tasks[i].Done = !s.tasks[i].Done
	}
	s.mu.Unlock()
	if i < 0 {
		return live.Invalid("id", "no such task")
	}
	s.redraw()
	return nil
}

func (s *store) redraw() {
	if err := s.w.Redraw(s.region.Name()); err != nil {
		log.Printf("templui: redraw: %v", err)
	}
}

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
	s := &store{w: w}
	s.region = w.Region("tasks", func() templ.Component { return components.TaskList(s.all()) })
	s.add("Read templUI's docs")
	s.add("Install the components")

	components.Add.Handle(w, func(_ *live.Session, in components.AddIn) (any, error) {
		s.add(in.Title)
		return nil, nil
	})
	components.Toggle.Handle(w, func(_ *live.Session, in components.ToggleIn) (any, error) {
		return nil, s.toggle(in.ID)
	})

	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("templui: %v (run `npm run generate` first)", err)
	}
	manifests.Notify(w)

	home := func(rw http.ResponseWriter, r *http.Request, failed ui.Option, status int) {
		templ.Handler(components.Home(s.region, failed), templ.WithStatus(status)).ServeHTTP(rw, r)
	}
	refused := func(rw http.ResponseWriter, r *http.Request, err error) {
		home(rw, r, ui.Failed(r, err), http.StatusUnprocessableEntity)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) { home(rw, r, nil, http.StatusOK) })
	mux.Handle("POST /add", components.Add.FormPOST(w,
		func(_ *http.Request, in components.AddIn) (string, error) {
			s.add(in.Title)
			return "/", nil
		}, refused))
	mux.Handle("POST /toggle", components.Toggle.FormPOST(w,
		func(_ *http.Request, in components.ToggleIn) (string, error) {
			return "/", s.toggle(in.ID)
		}, refused))
	w.Mount(mux, "/live")
	// public/ also holds templUI's script bundle under js/.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	log.Printf("templui: listening on %s", addr)
	if err := http.ListenAndServe(addr, templatic.App(w, manifests, mux)); err != nil {
		log.Fatal(err)
	}
}
