package main

import (
	"log"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"

	"examples/bootstrap/components"
)

// board holds the services and redraws the board region on every change.
type board struct {
	w      *live.World
	region *live.Region

	mu       sync.Mutex
	services []components.Service
}

var errNoService = live.Invalid("service", "no such service")

// newBoard declares the board region on w with a few services.
func newBoard(w *live.World) *board {
	now := time.Now()
	b := &board{w: w}
	for _, s := range [][2]string{{"api", "API"}, {"web", "Website"}, {"db", "Database"}, {"queue", "Queue"}} {
		b.services = append(b.services, components.Service{ID: s[0], Name: s[1], Status: "up", Since: now, LatencyMS: 40})
	}
	b.region = w.Region("board", func() templ.Component { return components.Board(b.all()) })
	return b
}

// all returns a copy of the services, in board order.
func (b *board) all() []components.Service {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.services)
}

// change applies fn to the service id and redraws the board.
func (b *board) change(id string, fn func(*components.Service)) error {
	b.mu.Lock()
	i := slices.IndexFunc(b.services, func(s components.Service) bool { return s.ID == id })
	if i < 0 {
		b.mu.Unlock()
		return errNoService
	}
	before := b.services[i].Status
	fn(&b.services[i])
	if b.services[i].Status != before {
		b.services[i].Since = time.Now()
	}
	b.mu.Unlock()
	if err := b.w.Redraw(b.region.Name()); err != nil {
		log.Printf("bootstrap: redraw: %v", err)
	}
	return nil
}

// set is Set's step, shared by both transports. Back to up resolves the incident.
func (b *board) set(in components.SetIn) error {
	return b.change(in.Service, func(s *components.Service) {
		s.Status = in.Status
		if in.Status == "up" {
			s.Incident = ""
		}
	})
}

// report is Report's step, shared by both transports.
func (b *board) report(in components.ReportIn) error {
	return b.change(in.Service, func(s *components.Service) {
		s.Status, s.Incident = "down", in.Note
	})
}

// measure sets a random service's latency every interval; it never returns.
func (b *board) measure(interval time.Duration) {
	for range time.Tick(interval) {
		all := b.all()
		id := all[rand.IntN(len(all))].ID
		_ = b.change(id, func(s *components.Service) { s.LatencyMS = 20 + rand.IntN(180) })
	}
}
