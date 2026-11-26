package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/ticker/components"
)

// newBoard declares n instruments on w, each with a price cell and a direction cell.
func newBoard(w *live.World, n int) *components.Board {
	b := &components.Board{
		Price: live.NewCellFamily(w, "px", ""),
		Dir:   live.NewCellFamily(w, "dir", ""),
		AsOf:  live.NewCell(w, "board.asof", ""),
	}
	for i := range n {
		// Keys go into topics, so they must match [A-Za-z0-9_.-]+ (issue #15).
		sym := fmt.Sprintf("T%03d", i+1)
		b.Symbols = append(b.Symbols, sym)
		b.Price.Get(sym).Set(formatCents(10_000 + 100*int64(i)))
	}
	return b
}

// feed moves rate prices a second, in batches every interval, and stamps
// AsOf after each batch; it never returns.
func feed(b *components.Board, rate int, interval time.Duration) {
	cents := make(map[string]int64, len(b.Symbols))
	for i, sym := range b.Symbols {
		cents[sym] = 10_000 + 100*int64(i)
	}
	perBatch := float64(rate) * interval.Seconds()
	owed := 0.0
	for range time.Tick(interval) {
		owed += perBatch
		for ; owed >= 1; owed-- {
			sym := b.Symbols[rand.IntN(len(b.Symbols))]
			step := rand.Int64N(41) - 20
			if step == 0 {
				continue
			}
			cents[sym] = max(cents[sym]+step, 1)
			b.Price.Get(sym).Set(formatCents(cents[sym]))
			// Publish the direction only when it flips: an unchanged value
			// would still be a frame to every viewer.
			dir := b.Dir.Get(sym)
			if next := direction(step); dir.Get() != next {
				dir.Set(next)
			}
		}
		b.AsOf.Set(time.Now().UTC().Format(time.RFC3339Nano))
	}
}

func direction(step int64) string {
	if step > 0 {
		return "up"
	}
	return "down"
}

func formatCents(c int64) string { return fmt.Sprintf("%d.%02d", c/100, c%100) }
