package components

import "github.com/nkapatos/templatic/live"

// AddIn is the input of Add, from the runtime and from the no-JS form.
type AddIn struct {
	N int `json:"n"`
}

// Validate keeps the step between -100 and 100.
func (in AddIn) Validate() error {
	if in.N < -100 || in.N > 100 {
		return live.Invalid("n", "pick a step between -100 and 100")
	}
	return nil
}

// Add adds N to the shared count.
var Add = live.NewCommand[AddIn]("count.add")
