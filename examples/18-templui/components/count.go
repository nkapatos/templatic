package components

import "github.com/nkapatos/templatic/live"

// AddIn is the input of the Add command and of its no-JS form POST: one
// shape, one Validate, two transports.
type AddIn struct {
	// A form field decodes by the field's type: "3" becomes 3.
	N int `json:"n"`
}

func (in AddIn) Validate() error {
	if in.N < -100 || in.N > 100 {
		return live.Invalid("n", "pick a step between -100 and 100")
	}
	return nil
}

// Add is declared once. The template and the handlers in main.go share it, so
// the name and the input type cannot drift apart.
var Add = live.NewCommand[AddIn]("count.add")
