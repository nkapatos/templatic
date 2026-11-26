package ui

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"
)

func TestDeclaredStreamMarkupMatchesStream(t *testing.T) {
	body := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, "x"); return err })
	d := live.DeclareStream[string]("t.log").Key(func(s string) string { return s }).
		Renders(func(string) templ.Component { return body }).ItemClass("it").Tail()
	r := func(c templ.Component) string {
		var b bytes.Buffer
		if err := c.Render(context.Background(), &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	want := r(Stream("t.log", StreamItem("k1", body, Class("it")), Tail()))
	if got := r(d.Items([]string{"k1"})); got != want {
		t.Fatalf("declared %s\nhelpers  %s", got, want)
	}
}
