package components

import "github.com/nkapatos/templatic/live"

// MaxPoints is the longest series the server keeps.
const MaxPoints = 60

// Windows are the window sizes the buttons offer, in points.
var Windows = []int{15, 30, 60}

// ChartProps are the props of the traffic-chart island.
type ChartProps struct {
	// Series is requests per second, oldest first, at most MaxPoints long.
	Series []int `json:"series"`
	// Window is how many of the latest points the chart shows.
	Window int `json:"window"`
}

// WindowIn is the input of SetWindow.
type WindowIn struct {
	Points int `json:"points"`
}

// Validate refuses a window outside 2..MaxPoints.
func (in WindowIn) Validate() error {
	if in.Points < 2 || in.Points > MaxPoints {
		return live.Invalid("points", "pick 2 to 60 points")
	}
	return nil
}

// SetWindow sets how many points every chart shows.
var SetWindow = live.NewCommand[WindowIn]("traffic.window")
