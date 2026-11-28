package main

import (
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/islandchart/components"
)

func TestSeriesKeepsTheLastPoints(t *testing.T) {
	w := live.NewWorld()
	tr := newTraffic(w)
	s := livetest.New(t, w)

	for i := range components.MaxPoints + 10 {
		tr.add(i)
	}
	var got []int
	if !s.Cell("traffic.series", &got) {
		t.Fatal("series was not pushed")
	}
	if len(got) != components.MaxPoints || got[0] != 10 || got[len(got)-1] != components.MaxPoints+9 {
		t.Fatalf("pushed series = %v, want 10..%d", got, components.MaxPoints+9)
	}
}

func TestSetWindow(t *testing.T) {
	w := live.NewWorld()
	newTraffic(w)
	s := livetest.New(t, w)

	if _, err := livetest.Call(s, components.SetWindow, components.WindowIn{Points: 15}); err != nil {
		t.Fatal(err)
	}
	var got int
	if !s.Cell("traffic.window", &got) || got != 15 {
		t.Fatalf("pushed window = %d, want 15", got)
	}
	_, err := livetest.Call(s, components.SetWindow, components.WindowIn{Points: 1})
	if errs := live.InputErrors(err); len(errs) != 1 || errs[0].Field != "points" {
		t.Fatalf("err = %v, want one error on points", err)
	}
}
