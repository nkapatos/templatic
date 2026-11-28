package components

// Place is one marker on the map.
type Place struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// MapProps are the place-map island's props.
type MapProps struct {
	Places []Place `json:"places"`
}
