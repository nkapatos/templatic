package livetest_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"
)

type entry struct{ ID, Text string }

func entries(w *live.World) *live.Collection[entry] {
	return live.NewCollection(w, "todo.log", func(e entry) string { return e.ID }).
		Renders(func(e entry) templ.Component { return text(e.Text) }).
		Elements("ul", "li")
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestShownCollectionRecordsUpsertsAndRemovals(t *testing.T) {
	w := live.NewWorld()
	log := entries(w)
	s := livetest.New(t, w)
	if page := render(t, log); page != `<ul data-tc-collection="todo.log" data-tc-ver="0"></ul>` {
		t.Fatalf("first paint: %s", page)
	}
	s.Show(log.Topic(), log.Version())
	_ = log.Upsert(entry{"a", "one"}, entry{"b", "two"})
	_ = log.Remove("a")
	var got []string
	for _, op := range s.Ops("todo.log") {
		got = append(got, op.Kind+":"+op.Key)
	}
	if strings.Join(got, ",") != "u:a,u:b,d:a" {
		t.Fatalf("ops %v", got)
	}
	if op := s.WaitOp("todo.log", "two", time.Second); op.HTML != `<li data-tc-item="b">two</li>` || op.Version != 2 {
		t.Fatalf("upsert %+v", op)
	}
}

func TestUnshownCollectionIsNotRecorded(t *testing.T) {
	w := live.NewWorld()
	log := entries(w)
	s := livetest.New(t, w)
	_ = log.Upsert(entry{"a", "one"})
	if ops := s.Ops("todo.log"); len(ops) != 0 {
		t.Fatalf("a session that does not show the collection got %+v", ops)
	}
}

func TestResumeCatchesUpWhatADroppedSessionMissed(t *testing.T) {
	w := live.NewWorld()
	log := entries(w)
	s := livetest.New(t, w)
	s.Show(log.Topic(), log.Version())
	_ = log.Upsert(entry{"a", "seen"})
	s.Drop()
	_ = log.Upsert(entry{"b", "missed"})
	if !s.Resume() {
		t.Fatal("a session dropped within the grace must resume")
	}
	ops := s.Ops("todo.log")
	if len(ops) != 2 || ops[1].Key != "b" || ops[1].Version != 2 {
		t.Fatalf("after resume: %+v", ops)
	}
}

func TestResumeAfterTheGraceIsRefused(t *testing.T) {
	w := live.NewWorld(live.WithResumeGrace(time.Millisecond))
	s := livetest.New(t, w)
	s.Drop()
	time.Sleep(20 * time.Millisecond)
	if s.Resume() {
		t.Fatal("a session past its grace must not resume")
	}
}

func TestCollectionFamilyMembers(t *testing.T) {
	w := live.NewWorld()
	threads := live.NewCollectionFamily(w, "chat.thread", func(e entry) string { return e.ID }).
		Renders(func(e entry) templ.Component { return text(e.Text) }).Tail()
	c1, err := threads.Get("c1")
	if err != nil || c1.Topic() != "chat.thread.c1" {
		t.Fatalf("member %v %v", c1, err)
	}
	if h := render(t, c1); h != `<div data-tc-collection="chat.thread.c1" data-tc-ver="0" data-tc-dir="tail"></div>` {
		t.Fatal(h)
	}
	if _, err := threads.Get("not ok"); err == nil {
		t.Fatal("an id built from data outside the topic grammar must be an error, not a panic")
	}
	_ = c1.Upsert(entry{"m", "hi"}, entry{"n", "there"})
	s := livetest.New(t, w)
	s.Show(c1.Topic(), 2)
	threads.Forget("c1")
	again, _ := threads.Get("c1")
	_ = again.Upsert(entry{"x", "new"})
	if again.Version() <= 2 {
		t.Fatalf("a recreated member starts above the forgotten one's version, got %d", again.Version())
	}
}
