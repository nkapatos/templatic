package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/tanstack/components"
)

func TestLogKeepsLogSizeNewestFirst(t *testing.T) {
	w := live.NewWorld()
	f := newLogFeed(w)
	f.add(3)
	got := f.rows.Items()
	if len(got) != components.LogSize || got[0].ID != components.LogSize+3 || got[len(got)-1].ID != 4 {
		t.Fatalf("kept %d rows, first %d, last %d; want %d, %d, 4", len(got), got[0].ID, got[len(got)-1].ID, components.LogSize, components.LogSize+3)
	}
}

func TestANewRowIsSentAlone(t *testing.T) {
	w := live.NewWorld()
	f := newLogFeed(w)
	s := livetest.New(t, w)
	// a page that rendered the log as it stands
	s.Show(components.LogTopic, f.rows.Version())

	f.add(1)
	ops := s.Ops(components.LogTopic)
	if len(ops) != 1 || ops[0].Kind != "u" || ops[0].Key != fmt.Sprintf("r%d", components.LogSize+1) {
		t.Fatalf("ops = %+v, want one upsert of the new row", ops)
	}
	// the dropped oldest row is not an op: the page trims by the same limit
	if n := len(ops[0].Value); n > 200 {
		t.Fatalf("the op carries %d bytes, want one row's worth", n)
	}
}

func TestAnOrderChangeIsSentAlone(t *testing.T) {
	w := live.NewWorld()
	orders := newOrders(w)
	s := livetest.New(t, w)
	s.Show(components.OrdersTopic, orders.Version())

	if _, err := orders.Update("o1000", func(o components.Order) components.Order { o.Status = "refunded"; return o }); err != nil {
		t.Fatal(err)
	}
	ops := s.Ops(components.OrdersTopic)
	if len(ops) != 1 || ops[0].Key != "o1000" || !strings.Contains(string(ops[0].Value), `"status":"refunded"`) {
		t.Fatalf("ops = %+v, want one upsert of o1000", ops)
	}
	if got := len(orders.Items()); got != orderCount {
		t.Fatalf("%d orders, want %d", got, orderCount)
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
