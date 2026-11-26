package widgets

import "time"

// CardProps feeds the user-card island.
type CardProps struct {
	Name    string             `json:"name"`
	Count   int64              `json:"count"`
	Active  bool               `json:"active,omitempty"`
	Ratio   float64            `json:"ratio"`
	Ignored string             `json:"-"`
	Inner   Address            `json:"inner"`
	Tags    []string           `json:"tags"`
	Lookup  map[string]Address `json:"lookup"`
	When    time.Time          `json:"when"`
	anon    string
}

type Address struct {
	City string `json:"city"`
	Zip  int    `json:"zip"`
}

// ChartProps feeds the lit variant island.
type ChartProps struct {
	Title   string    `json:"title"`
	Focus   string    `json:"focus"`
	Range   string    `json:"range"`
	Series  []float64 `json:"series"`
	Address Address   `json:"address"`
}
