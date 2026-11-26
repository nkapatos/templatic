package ui_test

import (
	"testing"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"
)

func TestCellAttrRendersThroughTheRenderRule(t *testing.T) {
	w := live.NewWorld()
	state := live.NewCell(w, "orb.state", "idle")
	got := ui.CellAttr(state, "data-state")
	want := templ.Attributes{"data-tc-a": "", "data-tc-a-data-state": "orb.state", "data-state": "idle"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %v, want %v (all: %v)", k, got[k], v, got)
		}
	}
	for _, v := range []any{"", nil} {
		a := ui.SessionCellAttr("orb.badge", "aria-label", v)
		if _, ok := a["aria-label"]; ok || a["data-tc-a-aria-label"] != "orb.badge" {
			t.Fatalf("%v: %v", v, a)
		}
	}
	if a := ui.SessionCellAttr("n", "value", 3); a["value"] != "3" {
		t.Fatalf("numbers render literally: %v", a)
	}

}

func TestCellAttrAllowlist(t *testing.T) {
	for _, ok := range []string{"data-state", "aria-busy", "class", "value", "Data-State"} {
		ui.SessionCellAttr("t", ok, "x")
	}
	for _, bad := range []string{"href", "src", "style", "onclick", "data-", "aria-", "data-tc-cell", "id", "data-x y", `data-"x`} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%q did not panic", bad)
				}
			}()
			ui.SessionCellAttr("t", bad, "x")
		}()
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("bad topic did not panic")
			}
		}()
		ui.SessionCellAttr("bad topic", "class", "x")
	}()
}
