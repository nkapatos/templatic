package components

import (
	"strconv"
	"time"

	"github.com/nkapatos/templatic/live"
)

// CountTopic is the shared count's cell topic.
const CountTopic = "count"

// AddIn is Add's input, from either transport.
type AddIn struct {
	N int `json:"n"`
}

// Validate refuses a step outside -100..100 and a zero step.
func (in AddIn) Validate() error {
	if in.N == 0 || in.N < -100 || in.N > 100 {
		return live.Invalid("n", "pick a step between -100 and 100, not 0")
	}
	return nil
}

// Add adds a step to the shared count and swaps the caller's own tally.
var Add = live.NewCommand[AddIn]("count.add").Swaps(TallyTopic)

// Entry is one line of the log: an add or a server tick.
type Entry struct {
	ID   int
	Kind string // "add" or "tick": the text's class
	Text string
	At   time.Time
}

// Key is the entry's item key.
func (e Entry) Key() string { return "e" + strconv.Itoa(e.ID) }

// LogTopic is the log's collection topic.
const LogTopic = "log"

// TallyTopic is the topic of the per-page tally region.
const TallyTopic = "tally"

// PageAdds is one page's own adds: how many and the last step.
type PageAdds struct {
	Count int
	Last  int
}
