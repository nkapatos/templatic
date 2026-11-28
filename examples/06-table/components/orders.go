package components

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nkapatos/templatic/live"
)

// Order is one row of the table.
type Order struct {
	ID       int
	Customer string
	Status   string
	Amount   int // cents
	Updated  time.Time
}

// TableTopic is the topic of the table session region.
const TableTopic = "orders"

// PageSize is how many rows one page shows.
const PageSize = 20

// Columns are the sortable columns, in display order; the first is the
// default sort, newest first.
var Columns = []struct{ Key, Label string }{
	{"updated", "Updated"},
	{"id", "Order"},
	{"customer", "Customer"},
	{"status", "Status"},
	{"amount", "Amount"},
}

// View is one viewer's place in the table: page, search, sort column and
// direction. It is the command's input and what the URL's query carries.
type View struct {
	Page int    `json:"page"`
	Q    string `json:"q"`
	Sort string `json:"sort"`
	Dir  string `json:"dir"`
}

// Result is a View with the rows it selects: the region's view model.
type Result struct {
	View  View
	Rows  []Order
	Total int // rows matching the search
	Pages int
	Fresh time.Time // rows updated at this time changed in the latest tick
}

// Navigate moves a viewer to a View; its handler swaps the table region and
// declares the View's URL.
var Navigate = live.NewCommand[View]("orders.view").Swaps(TableTopic)

// ParseView reads a View from a URL query; unknown or missing values fall
// back to the defaults.
func ParseView(q url.Values) View {
	page, err := strconv.Atoi(q.Get("page"))
	if err != nil {
		page = 1
	}
	return View{Page: page, Q: q.Get("q"), Sort: q.Get("sort"), Dir: q.Get("dir")}.Clean()
}

// Clean returns v with every field in range: page at least 1, q trimmed
// and at most 64 bytes, sort a known column, dir asc or desc (the
// column's default when empty). The upper bound of page depends on the
// data and is applied by the query.
func (v View) Clean() View {
	v.Page = max(v.Page, 1)
	v.Q = strings.TrimSpace(v.Q)
	if len(v.Q) > 64 {
		v.Q = strings.ToValidUTF8(v.Q[:64], "")
	}
	if !known(v.Sort) {
		v.Sort = Columns[0].Key
	}
	if v.Dir != "asc" && v.Dir != "desc" {
		v.Dir = defaultDir(v.Sort)
	}
	return v
}

// URL returns the page address of v, leaving out fields at their default.
func (v View) URL() string {
	q := url.Values{}
	if v.Page > 1 {
		q.Set("page", strconv.Itoa(v.Page))
	}
	if v.Q != "" {
		q.Set("q", v.Q)
	}
	if v.Sort != Columns[0].Key {
		q.Set("sort", v.Sort)
	}
	if v.Dir != defaultDir(v.Sort) {
		q.Set("dir", v.Dir)
	}
	if len(q) == 0 {
		return "/"
	}
	return "/?" + q.Encode()
}

// SortedBy returns v sorted by col from the first page: the column's
// default direction, or the other one when v is already sorted by col.
func (v View) SortedBy(col string) View {
	dir := defaultDir(col)
	if v.Sort == col {
		dir = map[string]string{"asc": "desc", "desc": "asc"}[v.Dir]
	}
	return View{Page: 1, Q: v.Q, Sort: col, Dir: dir}
}

// OnPage returns v at page n.
func (v View) OnPage(n int) View { v.Page = n; return v }

func known(col string) bool {
	for _, c := range Columns {
		if c.Key == col {
			return true
		}
	}
	return false
}

// Newest first is the useful order for the update time; every other column
// starts ascending.
func defaultDir(col string) string {
	if col == "updated" {
		return "desc"
	}
	return "asc"
}

func money(cents int) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }
