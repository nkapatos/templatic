package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/feed/components"
)

// shown is a session whose page shows room and the activity log, as a room
// page does.
func shown(t *testing.T, w *live.World, f *feed, room string) *livetest.Session {
	s := livetest.New(t, w)
	r := f.room(room)
	s.Show(r.Topic(), r.Version())
	s.Show(f.activity.Topic(), f.activity.Version())
	return s
}

func TestPostReachesPagesShowingTheRoom(t *testing.T) {
	w := live.NewWorld()
	f := newFeed(w)
	general, random := shown(t, w, f, "general"), shown(t, w, f, "random")

	if _, err := livetest.Call(general, components.Post, components.PostIn{Room: "general", Name: "ada", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	ops := general.Ops("chat.general")
	if len(ops) != 1 || ops[0].Kind != "u" || !strings.Contains(ops[0].HTML, `<li data-tc-item="m`) || !strings.Contains(ops[0].HTML, "hello") {
		t.Fatalf("chat.general = %+v, want one keyed <li> with the text", ops)
	}
	if got := random.Ops("chat.general"); len(got) != 0 {
		t.Fatalf("a page showing #random got #general's ops: %+v", got)
	}
	if log := general.Ops(components.ActivityTopic); len(log) == 0 || !strings.Contains(log[len(log)-1].HTML, `class="post"`) {
		t.Fatalf("activity = %+v, want the post last", log)
	}
}

func TestLikeUpdatesAndDeleteRemoves(t *testing.T) {
	w := live.NewWorld()
	f := newFeed(w)
	s := shown(t, w, f, "general")
	if err := f.post(components.PostIn{Room: "general", Name: "ada", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	key := f.room("general").Items()[0].Key()
	for range 2 {
		if _, err := livetest.Call(s, components.Like, components.MessageIn{Room: "general", ID: key}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := livetest.Call(s, components.Delete, components.MessageIn{Room: "general", ID: key}); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, op := range s.Ops("chat.general") {
		kinds = append(kinds, op.Kind+":"+op.Key)
	}
	want := "u:" + key + ",u:" + key + ",u:" + key + ",d:" + key
	if strings.Join(kinds, ",") != want {
		t.Fatalf("ops %v, want %s", kinds, want)
	}
	if ops := s.Ops("chat.general"); !strings.Contains(ops[2].HTML, "♥ 2") {
		t.Fatalf("second like renders %q", ops[2].HTML)
	}
	if _, err := livetest.Call(s, components.Like, components.MessageIn{Room: "general", ID: key}); err == nil {
		t.Fatal("liking a deleted message must be refused")
	}
}

func TestPostRefusesAnUnknownRoomAndEmptyText(t *testing.T) {
	w := live.NewWorld()
	newFeed(w)
	s := livetest.New(t, w)

	_, err := livetest.Call(s, components.Post, components.PostIn{Room: "attic", Name: "ada"})
	got := map[string]bool{}
	for _, ie := range live.InputErrors(err) {
		got[ie.Field] = true
	}
	if !got[""] || !got["text"] || len(got) != 2 {
		t.Fatalf("errors on %v, want the room (no field) and text", got)
	}
}

func TestARoomKeepsItsLastMessages(t *testing.T) {
	w := live.NewWorld()
	f := newFeed(w)
	for i := range keepMessages + 5 {
		if err := f.post(components.PostIn{Room: "general", Name: "ada", Text: "n" + strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}
	got := f.room("general").Items()
	if len(got) != keepMessages || got[0].Text != "n5" {
		t.Fatalf("kept %d messages from %q, want %d from n5", len(got), got[0].Text, keepMessages)
	}
}
