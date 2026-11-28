package main

import (
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/livevalue/components"
)

func TestFillCapsAndEmpty(t *testing.T) {
	w := live.NewWorld()
	level := newTank(w)
	s := livetest.New(t, w)

	for range 2 {
		if _, err := livetest.Call(s, components.Fill, components.FillIn{Amount: 50}); err != nil {
			t.Fatal(err)
		}
	}
	if got := level.Get(); got != components.Capacity {
		t.Fatalf("level = %d, want %d", got, components.Capacity)
	}
	if _, err := livetest.Call(s, components.Empty, struct{}{}); err != nil {
		t.Fatal(err)
	}
	var got int
	if !s.Cell("tank.level", &got) || got != 0 {
		t.Fatalf("pushed level = %d, want 0", got)
	}
}

func TestFillRefusesZero(t *testing.T) {
	w := live.NewWorld()
	newTank(w)
	s := livetest.New(t, w)
	_, err := livetest.Call(s, components.Fill, components.FillIn{Amount: 0})
	if errs := live.InputErrors(err); len(errs) != 1 || errs[0].Field != "amount" {
		t.Fatalf("err = %v, want one error on amount", err)
	}
}
