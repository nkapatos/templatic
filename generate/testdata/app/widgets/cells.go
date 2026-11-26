package widgets

import "github.com/nkapatos/templatic/live"

// RangeTopic is read by the bind check as a const.
const RangeTopic = "chart.range"

var Focus = live.DeclareSessionCell("chart.focus", "")

func register(w *live.World) { _ = live.NewCell[string](w, RangeTopic, "") }
