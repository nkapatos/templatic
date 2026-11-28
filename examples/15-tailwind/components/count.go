package components

import "github.com/nkapatos/templatic/live"

// AddIn is the input of the Add command and of its no-JS form POST.
type AddIn struct {
	N int `json:"n"`
}

// Validate refuses steps outside -100..100.
func (in AddIn) Validate() error {
	if in.N < -100 || in.N > 100 {
		return live.Invalid("n", "pick a step between -100 and 100")
	}
	return nil
}

// Add adds N to the count.
var Add = live.NewCommand[AddIn]("count.add")

// Goal is the count at which Level turns green.
const Goal = 10
