package components

import (
	"strings"
	"unicode/utf8"

	"github.com/nkapatos/templatic/live"
)

// Task is one entry of the shared task list.
type Task struct {
	ID    int
	Title string
	Done  bool
}

// AddIn is the input of the Add command and of its no-JS form POST.
type AddIn struct {
	Title string `json:"title"`
}

// Validate refuses an empty title and one longer than 40 characters.
func (in AddIn) Validate() error {
	t := strings.TrimSpace(in.Title)
	if t == "" {
		return live.Invalid("title", "give the task a title")
	}
	if utf8.RuneCountInString(t) > 40 {
		return live.Invalid("title", "keep it under 40 characters")
	}
	return nil
}

// Add adds a task to the list.
var Add = live.NewCommand[AddIn]("task.add")

// ToggleIn names the task whose Done flips.
type ToggleIn struct {
	ID int `json:"id"`
}

// Toggle marks a task done or open again.
var Toggle = live.NewCommand[ToggleIn]("task.toggle")
