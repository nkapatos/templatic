package components

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/nkapatos/templatic/live"
)

// Plans are the plans the sign-up form offers.
var Plans = []string{"free", "pro", "team"}

// Reserved are usernames Validate refuses; the client does not know them.
var Reserved = []string{"admin", "root", "support", "templatic"}

var usernameRE = regexp.MustCompile(`^[a-z0-9-]{3,20}$`)

// SignupProps are the props of the signup-form island.
type SignupProps struct {
	Plans []string `json:"plans"`
}

// SignupIn is Register's input, from the island's typed send or a plain form POST.
type SignupIn struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Plan     string `json:"plan"`
}

// Validate refuses a malformed or reserved username, an email without @ and an
// unknown plan, all at once. The island checks the same shapes while typing.
func (in SignupIn) Validate() error {
	var errs []error
	switch {
	case !usernameRE.MatchString(in.Username):
		errs = append(errs, live.Invalid("username", "3 to 20 of a-z, 0-9 and -"))
	case slices.Contains(Reserved, in.Username):
		errs = append(errs, live.Invalid("username", "that name is reserved"))
	}
	if at := strings.IndexByte(in.Email, '@'); at < 1 || at == len(in.Email)-1 {
		errs = append(errs, live.Invalid("email", "enter an email address"))
	}
	if !slices.Contains(Plans, in.Plan) {
		errs = append(errs, live.Invalid("plan", "pick a plan"))
	}
	return errors.Join(errs...)
}

// Register signs a user up; a taken username is refused by the handler.
var Register = live.NewCommand[SignupIn]("signup.register")
