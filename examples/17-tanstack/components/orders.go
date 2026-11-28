package components

import (
	"strconv"

	"github.com/nkapatos/templatic/live"
)

// Order is one row of the orders table.
type Order struct {
	ID       int    `json:"id"`
	Customer string `json:"customer"`
	Status   string `json:"status"`
	// Amount is in cents.
	Amount int `json:"amount"`
	// Placed is the order date, YYYY-MM-DD.
	Placed string `json:"placed"`
}

// Key is the order's item key in the orders collection.
func (o Order) Key() string { return "o" + strconv.Itoa(o.ID) }

// OrdersTopic is the orders collection's topic.
const OrdersTopic = "orders.all"

// TableProps are the props of the order-table island.
type TableProps struct {
	Orders live.Snapshot[Order] `json:"orders"`
}
