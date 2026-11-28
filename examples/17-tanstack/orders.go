package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/nkapatos/templatic/live"

	"examples/tanstack/components"
)

const orderCount = 500

var (
	customers = []string{"Acme", "Globex", "Initech", "Umbrella", "Hooli", "Stark", "Wayne", "Tyrell", "Cyberdyne", "Soylent"}
	statuses  = []string{"new", "paid", "shipped", "refunded"}
)

// newOrders declares the orders collection on w, filled with orderCount
// made-up orders in id order.
func newOrders(w *live.World) *live.Collection[components.Order] {
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
	c := live.NewCollection(w, components.OrdersTopic, components.Order.Key).Tail()
	if err := c.Reset(orders); err != nil {
		panic(err)
	}
	return c
}

// changeOrders moves a few orders to another status every interval, one op
// each; it never returns.
func changeOrders(orders *live.Collection[components.Order], interval time.Duration) {
	for range time.Tick(interval) {
		for range 3 {
			key := "o" + strconv.Itoa(1000+rand.IntN(orderCount))
			if _, err := orders.Update(key, func(o components.Order) components.Order {
				o.Status = statuses[rand.IntN(len(statuses))]
				return o
			}); err != nil {
				log.Printf("tanstack: orders: %v", err)
			}
		}
	}
}
