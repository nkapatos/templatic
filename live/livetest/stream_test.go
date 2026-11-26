package livetest_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"
)

type entry struct{ ID, Text string }

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestDeclaredStreamPaintAndAppendAgree(t *testing.T) {
	log := live.DeclareStream[entry]("todo.log").
		Key(func(e entry) string { return e.ID }).
		Renders(func(e entry) templ.Component { return text(e.Text) }).
		ItemClass("log-item")
	w := live.NewWorld()
	log.Bind(w, func() []entry { return nil })
	s := livetest.New(t, w)

	page := render(t, log.Items([]entry{{"a", "one"}}))
	if page != `<div data-tc-stream="todo.log"><div data-tc-item="a" class="log-item">one</div></div>` {
		t.Fatalf("first paint: %s", page)
	}
	if r := render(t, log.Region(nil)); r != `<div data-tc-region="todo.log"><div data-tc-stream="todo.log"></div></div>` {
		t.Fatalf("region: %s", r)
	}
	if err := log.Append(w, entry{"b", "two"}); err != nil {
		t.Fatal(err)
	}
	got := s.Appends("todo.log")
	if len(got) != 1 || got[0] != render(t, log.Item(entry{"b", "two"})) {
		t.Fatalf("append %q != Item", got)
	}
}

func TestStreamFamilyMembers(t *testing.T) {
	thread := live.DeclareStreamFamily[entry]("chat.thread").Key(func(e entry) string { return e.ID }).Tail()
	thread.Renders(func(e entry) templ.Component { return text(e.Text) })
	m := thread.For("c1")
	if m.Topic() != "chat.thread.c1" {
		t.Fatal(m.Topic())
	}
	if h := render(t, m.Items(nil)); !strings.Contains(h, `data-tc-stream="chat.thread.c1" data-tc-dir="tail"`) {
		t.Fatal(h)
	}
}
