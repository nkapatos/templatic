package components

import (
	"time"

	"github.com/nkapatos/templatic/live"
)

// Task is one entry of the shared queue.
type Task struct {
	ID    string
	Title string
}

// MoveIn moves the task ID to position To (0-based) of the queue.
type MoveIn struct {
	ID string `json:"id"`
	To int    `json:"to"`
}

// Move is raised by the up/down forms and, after a drag, by the sortable hook.
var Move = live.NewCommand[MoveIn]("queue.move")

// Stats are the numbers the locale hook formats in the browser.
type Stats struct {
	Revenue   float64
	Currency  string
	Visitors  int
	Reordered time.Time
	Now       time.Time
}
