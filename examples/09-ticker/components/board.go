package components

import "github.com/nkapatos/templatic/live"

// Board is the page's live state: a price and a direction per symbol, and
// the time of the last batch.
type Board struct {
	Symbols []string
	Price   *live.CellFamily[string]
	Dir     *live.CellFamily[string]
	AsOf    *live.Cell[string]
}
