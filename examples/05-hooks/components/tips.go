package components

import (
	"fmt"

	"github.com/nkapatos/templatic/live"
)

// Tip is one card of the tips region.
type Tip struct {
	Title, Body string
}

var tips = []Tip{
	{"Escape closes", "Open the More menu, then press Escape: it closes and focus returns to its button."},
	{"Click outside", "An open menu closes when you click anywhere else on the page."},
	{"Swapped in", "This card came from the server. Its menu is a new element, and the menu hook mounted on it."},
}

// TipIn is the input of ShowTip: the index of the tip to show.
type TipIn struct {
	N int `json:"n"`
}

func (in TipIn) Validate() error {
	if in.N < 0 || in.N >= len(tips) {
		return live.Invalid("n", fmt.Sprintf("pick a tip between 0 and %d", len(tips)-1))
	}
	return nil
}

// tipTopic is a constant, not TipCard.Topic(): the card renders a ShowTip
// link, so the two declarations referring to each other is an init cycle.
const tipTopic = "tip"

// TipCard is the session region the tips card lives in.
var TipCard = live.DeclareSessionRegion[int](tipTopic).Renders(tipCard)

// ShowTip swaps the tips card to another tip, for this page only.
var ShowTip = live.NewCommand[TipIn]("tip.show").Swaps(tipTopic)

// TipIndex parses a ?tip= value, falling back to the first tip.
func TipIndex(s string) int {
	var n int
	if _, err := fmt.Sscan(s, &n); err != nil || (TipIn{n}).Validate() != nil {
		return 0
	}
	return n
}

// TipURL is the page that shows tip n, the no-JS target of ShowTip.
func TipURL(n int) string { return fmt.Sprintf("/?tip=%d", n) }

func next(n int) int { return (n + 1) % len(tips) }
