package main

import (
	"net/http"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/livevalue/components"
)

// newTank declares the tank's level on w and mounts the buttons' commands
// on mux; without JavaScript each press is a POST answered with a redirect
// back to the page.
func newTank(w *live.World, mux *http.ServeMux) *live.Cell[int] {
	level := live.NewCell(w, "tank.level", 50)
	mux.Handle("POST /fill", components.Fill.Form(w, func(_ *http.Request, _ *live.Session, in components.FillIn) (string, error) {
		level.Update(func(v int) int { return min(v+in.Amount, components.Capacity) })
		return "/", nil
	}, nil))
	mux.Handle("POST /empty", components.Empty.Form(w, func(*http.Request, *live.Session, struct{}) (string, error) {
		level.Set(0)
		return "/", nil
	}, nil))
	return level
}

// drain lowers level by one every interval, down to zero; it never returns.
func drain(level *live.Cell[int], interval time.Duration) {
	for range time.Tick(interval) {
		level.Update(func(v int) int { return max(v-1, 0) })
	}
}
