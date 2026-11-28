package items

import "slices"

// Item is one entry of the list; the catalog is fixed, only likes change.
type Item struct {
	ID   string
	Name string
	Note string
}

var catalog = []Item{
	{ID: "1", Name: "Kettle", Note: "Boils a litre in two minutes."},
	{ID: "2", Name: "Teapot", Note: "Cast iron, holds four cups."},
	{ID: "3", Name: "Strainer", Note: "Fine mesh, fits any cup."},
}

// find returns the item with id, if there is one.
func find(id string) (Item, bool) {
	i := slices.IndexFunc(catalog, func(it Item) bool { return it.ID == id })
	if i < 0 {
		return Item{}, false
	}
	return catalog[i], true
}
