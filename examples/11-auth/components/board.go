package components

import (
	"strings"
	"unicode/utf8"

	"github.com/nkapatos/templatic/live"
)

// User is a signed-in account; Admin decides which commands it may run.
type User struct {
	Name  string
	Admin bool
}

// Role names the user's role for display.
func (u User) Role() string {
	if u.Admin {
		return "admin"
	}
	return "viewer"
}

// Entry is one line of the activity log.
type Entry struct {
	Who  string
	What string
}

// NoticeIn is PostNotice's input.
type NoticeIn struct {
	Text string `json:"text"`
}

// Validate refuses an empty or overlong notice.
func (in NoticeIn) Validate() error {
	switch t := strings.TrimSpace(in.Text); {
	case t == "":
		return live.Invalid("text", "write a notice")
	case utf8.RuneCountInString(t) > 120:
		return live.Invalid("text", "keep it under 120 characters")
	}
	return nil
}

// WaveIn is Wave's input; the command carries no fields.
type WaveIn struct{}

// PostNotice replaces the notice every page shows; admins only.
var PostNotice = live.NewCommand[NoticeIn]("notice.post")

// Wave adds a line to the activity log; any signed-in user.
var Wave = live.NewCommand[WaveIn]("wave")
