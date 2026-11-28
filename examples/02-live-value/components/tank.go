package components

import "github.com/nkapatos/templatic/live"

// Capacity is the most the tank holds.
const Capacity = 100

// FillIn is the input of Fill: how much one press adds.
type FillIn struct {
	Amount int `json:"amount"`
}

// Validate refuses an amount outside 1..Capacity.
func (in FillIn) Validate() error {
	if in.Amount < 1 || in.Amount > Capacity {
		return live.Invalid("amount", "fill between 1 and 100")
	}
	return nil
}

// Fill adds Amount to the tank, up to Capacity.
var Fill = live.NewCommand[FillIn]("tank.fill")

// Empty sets the tank to zero.
var Empty = live.NewCommand[struct{}]("tank.empty")
