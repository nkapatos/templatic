package ui

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/live"
)

type markIn struct {
	ID    string    `json:"id"`
	Step  int       `json:"step"`
	Done  bool      `json:"done"`
	Rank  *int      `json:"rank"`
	Tags  []string  `json:"tags"`
	Due   time.Time `json:"due"`
	Ratio float64   `json:"ratio"`
}

var hiddenInput = regexp.MustCompile(`<input type="hidden" name="([^"]*)" value="([^"]*)">`)

func TestButtonPostToRoundTripsItsInputThroughTheFormHandler(t *testing.T) {
	cmd := live.NewCommand[markIn]("task.mark")
	rank := 3
	in := markIn{ID: `a"<b`, Step: -2, Done: true, Rank: &rank, Tags: []string{"x", "y z"}, Due: time.Date(2026, 11, 28, 9, 30, 0, 0, time.UTC), Ratio: 0.25}
	label := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, "Mark"); return err })
	got := renderString(t, templ.WithChildren(context.Background(), label), Button(cmd, in, PostTo("/tasks/mark"), Class("primary")))

	for _, want := range []string{`<form method="POST" action="/tasks/mark" data-tc-form="task.mark">`, `<button type="submit" class="primary">Mark</button></form>`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	fields := url.Values{}
	for _, m := range hiddenInput.FindAllStringSubmatch(got, -1) {
		fields.Add(html.UnescapeString(m[1]), html.UnescapeString(m[2]))
	}
	var decoded markIn
	h := cmd.Form(live.NewWorld(), func(_ *http.Request, _ *live.Session, in markIn) (string, error) { decoded = in; return "/", nil }, nil)
	r := httptest.NewRequest("POST", "/tasks/mark", strings.NewReader(fields.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusSeeOther || !reflect.DeepEqual(decoded, in) {
		t.Fatalf("status %d: decoded %+v, want %+v", rec.Code, decoded, in)
	}
}

func TestButtonPostToRefusesAnInputNoFormCanCarry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a map field must panic")
		}
	}()
	Button(live.NewCommand[struct {
		M map[string]int `json:"m"`
	}]("bad.input"), struct {
		M map[string]int `json:"m"`
	}{}, PostTo("/x"))
}
