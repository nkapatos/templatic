package main

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/table/components"
)

func TestParseView(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  components.View
	}{
		{"", components.View{Page: 1, Sort: "updated", Dir: "desc"}},
		{"page=3&sort=id", components.View{Page: 3, Sort: "id", Dir: "asc"}},
		{"page=-2&sort=amount&dir=desc&q=+acme+", components.View{Page: 1, Q: "acme", Sort: "amount", Dir: "desc"}},
		{"page=x&sort=bogus&dir=up", components.View{Page: 1, Sort: "updated", Dir: "desc"}},
	} {
		q, _ := url.ParseQuery(tc.query)
		got := components.ParseView(q)
		if got != tc.want {
			t.Errorf("ParseView(%q) = %+v, want %+v", tc.query, got, tc.want)
		}
		u, _ := url.Parse(got.URL())
		if back := components.ParseView(u.Query()); back != got {
			t.Errorf("ParseView(%q) = %+v, want it back from %q", got.URL(), back, tc.query)
		}
	}
}

func TestUpdateKeepsEachSessionsView(t *testing.T) {
	w := live.NewWorld()
	tb := newTable(w, newStore(300, time.Now()))
	a := livetest.New(t, w)
	b := livetest.New(t, w, livetest.Request(httptest.NewRequest("GET", "/?sort=amount&dir=desc", nil)))

	if _, err := livetest.Call(a, components.Navigate, components.View{Page: 3, Sort: "id"}); err != nil {
		t.Fatal(err)
	}
	a.Reset()
	if err := tb.update(time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := a.WaitSwap(components.TableTopic, "Page 3 of", time.Second); !strings.Contains(got, `aria-sort="ascending" class="col-id"`) {
		t.Error("session a lost its sort by id")
	}
	if got := b.WaitSwap(components.TableTopic, "Page 1 of", time.Second); !strings.Contains(got, `aria-sort="descending" class="col-amount"`) {
		t.Error("session b lost the sort its page was loaded with")
	}
}
