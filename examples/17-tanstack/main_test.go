package main

import (
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/tanstack/components"
)

func TestLogKeepsLogSizeNewestFirst(t *testing.T) {
	w := live.NewWorld()
	f := newLogFeed(w)
	s := livetest.New(t, w)

	f.add(3)
	var got []components.Row
	if !s.Cell("log.rows", &got) {
		t.Fatal("rows were not pushed")
	}
	if len(got) != components.LogSize || got[0].ID != components.LogSize+3 || got[len(got)-1].ID != 4 {
		t.Fatalf("pushed %d rows, first %d, last %d; want %d, %d, 4", len(got), got[0].ID, got[len(got)-1].ID, components.LogSize, components.LogSize+3)
	}
}

func TestRegisterRefusesOnTheField(t *testing.T) {
	w := live.NewWorld()
	newSignups(w, "ada")
	s := livetest.New(t, w)

	for _, tc := range []struct{ name, want string }{
		{"admin", "that name is reserved"}, // Validate
		{"ada", "that name is taken"},      // the handler
	} {
		_, err := livetest.Call(s, components.Register, components.SignupIn{Username: tc.name, Email: "a@b.c", Plan: "pro"})
		if errs := live.InputErrors(err); len(errs) != 1 || errs[0].Field != "username" || errs[0].Message != tc.want {
			t.Errorf("%s: err = %v, want %q on username", tc.name, err, tc.want)
		}
	}
	if _, err := livetest.Call(s, components.Register, components.SignupIn{Username: "kim", Email: "k@b.c", Plan: "free"}); err != nil {
		t.Fatal(err)
	}
	var taken string
	if !s.Cell("signup.taken", &taken) || taken != "ada, kim" {
		t.Fatalf("taken = %q, want %q", taken, "ada, kim")
	}
}
