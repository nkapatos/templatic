package components

import (
	"net/url"
	"strings"

	"github.com/nkapatos/templatic/live"
)

// Item is one entry of the inbox.
type Item struct {
	ID, Title, Body string
	Done            bool
}

// Items is the inbox: fixed data, the same for every viewer.
var Items = []Item{
	{"1", "Renew the TLS certificate", "The certificate for the staging host expires on Friday.", false},
	{"2", "Answer the vendor about invoices", "They need the purchase order number before the end of the month.", false},
	{"3", "Review the onboarding guide", "Two new people start next week; the setup steps are out of date.", false},
	{"4", "Book the team offsite", "Pick one of the three venues and confirm the dates.", false},
	{"5", "Migrate the build to the new runners", "The old runners are switched off at the end of the quarter.", true},
	{"6", "Archive last year's tickets", "Everything closed before January can go to cold storage.", true},
	{"7", "Rotate the deploy keys", "Done for production; staging followed a day later.", true},
}

// InboxTopic is the topic of the inbox session region.
const InboxTopic = "inbox"

// View is one viewer's place in the inbox: the tab, the search and the open
// item. It is the command's input and what the URL's query carries.
type View struct {
	Tab  string `json:"tab"`
	Q    string `json:"q"`
	Open string `json:"open"`
}

// Navigate moves a viewer to a View; its handler swaps the inbox region and
// declares the View's URL.
var Navigate = live.NewCommand[View]("inbox.view").Swaps(InboxTopic)

// ParseView reads a View from a URL query; unknown or missing values fall back
// to the defaults.
func ParseView(q url.Values) View {
	return View{Tab: q.Get("tab"), Q: q.Get("q"), Open: q.Get("open")}.Clean()
}

// Clean returns v with every field in range: tab is open or done, q is
// trimmed and at most 64 bytes, open names an existing item or is empty.
func (v View) Clean() View {
	if v.Tab != "done" {
		v.Tab = "open"
	}
	v.Q = strings.TrimSpace(v.Q)
	if len(v.Q) > 64 {
		v.Q = strings.ToValidUTF8(v.Q[:64], "")
	}
	if _, ok := Find(v.Open); !ok {
		v.Open = ""
	}
	return v
}

// URL returns the page address of v, leaving out fields at their default.
func (v View) URL() string {
	q := url.Values{}
	if v.Tab != "open" {
		q.Set("tab", v.Tab)
	}
	if v.Q != "" {
		q.Set("q", v.Q)
	}
	if v.Open != "" {
		q.Set("open", v.Open)
	}
	if len(q) == 0 {
		return "/"
	}
	return "/?" + q.Encode()
}

// WithTab returns v on tab, keeping the search and closing the item.
func (v View) WithTab(tab string) View { return View{Tab: tab, Q: v.Q} }

// WithOpen returns v with item id open.
func (v View) WithOpen(id string) View { v.Open = id; return v }

// Find returns the item with id.
func Find(id string) (Item, bool) {
	for _, it := range Items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

// Listed returns the items on tab whose title contains q, case ignored.
func Listed(tab, q string) []Item {
	var out []Item
	for _, it := range Items {
		if it.Done != (tab == "done") {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Title), strings.ToLower(q)) {
			continue
		}
		out = append(out, it)
	}
	return out
}
