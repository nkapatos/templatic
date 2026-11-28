package main

import (
	"math/rand/v2"
	"slices"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/islandchart/components"
)

// traffic is the world state the chart shows.
type traffic struct {
	series *live.Cell[[]int]
	window *live.Cell[int]
}

// newTraffic declares the traffic cells on w and registers SetWindow.
func newTraffic(w *live.World) traffic {
	t := traffic{
		series: live.NewCell(w, "traffic.series", []int{}),
		window: live.NewCell(w, "traffic.window", 30),
	}
	components.SetWindow.Handle(w, func(s *live.Session, in components.WindowIn) (any, error) {
		t.window.Set(in.Points)
		return nil, nil
	})
	return t
}

// props is the island's props at render time.
func (t traffic) props() components.ChartProps {
	return components.ChartProps{Series: t.series.Get(), Window: t.window.Get()}
}

// sample appends a made-up requests/sec reading every interval, keeping the
// last MaxPoints; it never returns.
func (t traffic) sample(interval time.Duration) {
	rps := 120
	for range time.Tick(interval) {
		rps = max(0, rps+rand.IntN(41)-20)
		t.add(rps)
	}
}

// add appends one reading to the series, dropping the oldest past MaxPoints.
func (t traffic) add(rps int) {
	t.series.Update(func(old []int) []int {
		// A new slice: the old one may still be read by a render.
		return slices.Concat(old[max(0, len(old)-components.MaxPoints+1):], []int{rps})
	})
}
