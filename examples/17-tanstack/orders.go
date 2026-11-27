package main

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/tanstack/components"
)

const orderCount = 500

var (
	customers = []string{"Acme", "Globex", "Initech", "Umbrella", "Hooli", "Stark", "Wayne", "Tyrell", "Cyberdyne", "Soylent"}
	statuses  = []string{"new", "paid", "shipped", "refunded"}
)

// newOrders declares the orders cell on w, filled with orderCount made-up orders.
func newOrders(w *live.World) *live.Cell[[]components.Order] {
	r := rand.New(rand.NewPCG(17, 17))
	orders := make([]components.Order, orderCount)
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range orders {
		orders[i] = components.Order{
			ID:       1000 + i,
			Customer: fmt.Sprintf("%s %d", customers[r.IntN(len(customers))], 1+r.IntN(9)),
			Status:   statuses[r.IntN(len(statuses))],
			Amount:   500 + r.IntN(250_000),
			Placed:   day.AddDate(0, 0, r.IntN(330)).Format(time.DateOnly),
		}
	}
	return live.NewCell(w, "orders.all", orders)
}

// changeOrders moves a few orders to another status every interval; it never returns.
func changeOrders(orders *live.Cell[[]components.Order], interval time.Duration) {
	for range time.Tick(interval) {
		orders.Update(func(old []components.Order) []components.Order {
			next := slices.Clone(old)
			for range 3 {
				o := &next[rand.IntN(len(next))]
				o.Status = statuses[rand.IntN(len(statuses))]
			}
			return next
		})
	}
}
