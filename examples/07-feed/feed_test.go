package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/live/livetest"

	"examples/feed/components"
)

func TestPostAppendsToTheRoom(t *testing.T) {
	w := live.NewWorld()
	newFeed(w)
	s := livetest.New(t, w)

	if _, err := livetest.Call(s, components.Post, components.PostIn{Room: "general", Name: "ada", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	items := s.Appends(components.Chat.Topic("general"))
	if len(items) != 1 || !strings.Contains(items[0], `data-tc-item="m`) || !strings.Contains(items[0], "hello") {
		t.Fatalf("chat.general appends = %q, want one keyed item with the text", items)
	}
	if got := s.Appends(components.Chat.Topic("random")); len(got) != 0 {
		t.Fatalf("chat.random appends = %q, want none", got)
	}
	// The session's own connect is the first event.
	if log := s.Appends(components.ActivityTopic); len(log) != 2 || !strings.Contains(log[1], `class="post"`) {
		t.Fatalf("activity appends = %q, want the join and the post", log)
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
	got := f.messages("general")
	if len(got) != keepMessages || got[0].Text != "n5" {
		t.Fatalf("kept %d messages from %q, want %d from n5", len(got), got[0].Text, keepMessages)
	}
}
