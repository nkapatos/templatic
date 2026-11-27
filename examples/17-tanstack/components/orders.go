package components

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

// TableProps are the props of the order-table island.
type TableProps struct {
	Orders []Order `json:"orders"`
}
