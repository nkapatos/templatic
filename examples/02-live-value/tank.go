package main

import (
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/livevalue/components"
)

// newTank declares the tank's level on w and registers the commands that change it.
func newTank(w *live.World) *live.Cell[int] {
	level := live.NewCell(w, "tank.level", 50)
	components.Fill.Handle(w, func(s *live.Session, in components.FillIn) (any, error) {
		level.Update(func(v int) int { return min(v+in.Amount, components.Capacity) })
		return nil, nil
	})
	components.Empty.Handle(w, func(s *live.Session, _ struct{}) (any, error) {
		level.Set(0)
		return nil, nil
	})
	return level
}

// drain lowers level by one every interval, down to zero; it never returns.
func drain(level *live.Cell[int], interval time.Duration) {
	for range time.Tick(interval) {
		level.Update(func(v int) int { return max(v-1, 0) })
	}
}
