package components

import (
	"slices"

	"github.com/nkapatos/templatic/live"
)

// LogSize is how many rows the log keeps; the oldest drop off as rows arrive.
const LogSize = 10_000

// Rates are the add rates the log page offers, in rows per second.
var Rates = []int{0, 1, 5, 20}

// Row is one log line.
type Row struct {
	ID    int    `json:"id"`
	At    string `json:"at"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

// LogProps are the props of the log-list island.
type LogProps struct {
	// Rows is the whole log, newest first, at most LogSize long.
	Rows []Row `json:"rows"`
}

// RateIn is the input of SetRate.
type RateIn struct {
	PerSecond int `json:"perSecond"`
}

// Validate refuses a rate the page does not offer.
func (in RateIn) Validate() error {
	if !slices.Contains(Rates, in.PerSecond) {
		return live.Invalid("perSecond", "pick 0, 1, 5 or 20 rows a second")
	}
	return nil
}

// SetRate sets how many rows a second the server adds to the log.
var SetRate = live.NewCommand[RateIn]("log.rate")
