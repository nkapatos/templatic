package live

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
)

func itemComp(label string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<p>"+label+"</p>")
		return err
	})
}

func TestAppendBurstDeliversEveryItemInOrder(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	r := openStream(t, ts, "burst")
	defer r.Close()
	r.discardUntilReady(t)

	const n = 50
	for i := 0; i < n; i++ {
		if err := w.Append("drops.list", itoa(int64(i)), itemComp("item "+itoa(int64(i)))); err != nil {
			t.Fatal(err)
		}
	}

	var items []streamItem
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(items) < n {
		frame, ok := r.next(t, 200*time.Millisecond)
		if !ok {
			continue
		}
		if !strings.Contains(frame, `"kind":"append"`) {
			continue
		}
		var got struct {
			Items []streamItem `json:"items"`
		}
		idx := strings.Index(frame, "data: ")
		if err := json.Unmarshal([]byte(frame[idx+6:]), &got); err != nil {
			t.Fatalf("bad append frame %q: %v", frame, err)
		}
		items = append(items, got.Items...)
	}
	if len(items) != n {
		t.Fatalf("burst lost items: got %d, want %d (%v)", len(items), n, items)
	}
	for i, it := range items {
		if it.K != itoa(int64(i)) {
			t.Fatalf("append order broken at %d: key %q (items %v)", i, it.K, items)
		}
		if !strings.Contains(it.HTML, "item "+itoa(int64(i))) {
			t.Fatalf("append item %d body wrong: %q", i, it.HTML)
		}
		if !strings.Contains(it.HTML, `<div data-tc-item="`+it.K+`">`) {
			t.Fatalf("append item %d missing keyed wrapper: %q", i, it.HTML)
		}
	}
}

func TestAppendBurstCoalescesToOneFrame(t *testing.T) {
	// A wide coalesce window so the test does not depend on machine speed (-race).
	w := NewWorld(WithCoalesceWindow(250 * time.Millisecond))
	const window = 250 * time.Millisecond
	ts := serveWorld(t, w)
	r := openStream(t, ts, "burst-coalesce")
	defer r.Close()
	r.discardUntilReady(t)

	for i := 0; i < 20; i++ {
		if err := w.Append("drops.list", itoa(int64(i)), itemComp("x")); err != nil {
			t.Fatal(err)
		}
	}

	frames := 0
	deadline := time.Now().Add(window + 2*time.Second)
	for time.Now().Before(deadline) {
		frame, ok := r.next(t, window+500*time.Millisecond)
		if !ok {
			break
		}
		if strings.Contains(frame, `"kind":"append"`) {
			frames++
			var got struct {
				Items []streamItem `json:"items"`
			}
			idx := strings.Index(frame, "data: ")
			if err := json.Unmarshal([]byte(frame[idx+6:]), &got); err != nil {
				t.Fatalf("bad append frame: %q", frame)
			}
			if len(got.Items) != 20 {
				t.Fatalf("coalesced frame carries %d items, want 20", len(got.Items))
			}
		}
	}
	if frames != 1 {
		t.Fatalf("burst split into %d append frames, want exactly 1", frames)
	}
}

func TestCoalescerKeepsSwapAndAppendsSeparate(t *testing.T) {
	mkItems := func(keys ...string) []byte {
		items := make([]streamItem, 0, len(keys))
		for _, k := range keys {
			items = append(items, streamItem{K: k, HTML: "<p>" + k + "</p>"})
		}
		b, err := json.Marshal(struct {
			Items []streamItem `json:"items"`
		}{Items: items})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	swapBody, _ := json.Marshal(struct {
		HTML string `json:"html"`
	}{HTML: "<p>full render</p>"})

	c := coalescer{}
	c.add(event{kind: KindSwap, topic: "drops.list", body: swapBody})
	c.add(event{kind: KindAppend, topic: "drops.list", body: mkItems("a")})
	c.add(event{kind: KindAppend, topic: "drops.list", body: mkItems("b", "c")})

	evs := c.events()
	if len(evs) != 2 {
		t.Fatalf("want swap + merged append, got %d events", len(evs))
	}
	if evs[0].kind != KindSwap || !strings.Contains(string(evs[0].body), "full render") {
		t.Fatalf("swap lost or reordered: %+v", evs[0])
	}
	var merged struct {
		Items []streamItem `json:"items"`
	}
	if err := json.Unmarshal(evs[1].body, &merged); err != nil {
		t.Fatalf("merged append body bad: %v %s", err, evs[1].body)
	}
	items := merged.Items
	var keys []string
	for _, it := range items {
		keys = append(keys, it.K)
	}
	if strings.Join(keys, ",") != "a,b,c" {
		t.Fatalf("merged items out of order: %v", keys)
	}
}

func TestStreamReplayOnReconnect(t *testing.T) {
	w := NewWorld()
	w.Stream("drops.list", func() templ.Component {
		return itemComp("timeline full truth")
	})

	ts := serveWorld(t, w)

	fresh := openStream(t, ts, "fresh-page")
	defer fresh.Close()
	fresh.discardUntilReady(t)
	if frame, ok := fresh.next(t, 150*time.Millisecond); ok {
		t.Fatalf("fresh connect received catch-up: %q", frame)
	}

	re := openStreamWithHeaders(t, ts, "fresh-page", map[string]string{"Last-Event-ID": "7"})
	defer re.Close()
	re.discardUntilReady(t)
	catchup, ok := re.next(t, 2*time.Second)
	if !ok || !strings.Contains(catchup, `"kind":"swap"`) {
		t.Fatalf("reconnect never received the stream catch-up swap: %q ok=%v", catchup, ok)
	}
	if !strings.Contains(catchup, `"topic":"drops.list"`) ||
		!strings.Contains(catchup, "timeline full truth") {
		t.Fatalf("catch-up swap wrong: %q", catchup)
	}
}

func TestAppendValidatesTopicAndKey(t *testing.T) {
	w := NewWorld()
	if err := w.Append("drops.list", "bad key!<", itemComp("x")); !errors.Is(err, errTopic) {
		t.Fatalf("bad key: got %v, want the topic grammar error", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on malformed append topic")
		}
	}()
	_ = w.Append("bad topic!", "d-1", itemComp("x"))
}

func TestStreamDeclBadKeyErrors(t *testing.T) {
	w := NewWorld()
	d := DeclareStream[string]("t.bad").Key(func(s string) string { return s }).Renders(itemComp)
	if err := d.Append(w, "a b"); !errors.Is(err, errTopic) {
		t.Fatalf("Append: %v", err)
	}
	if err := d.Item("a b").Render(context.Background(), io.Discard); !errors.Is(err, errTopic) {
		t.Fatalf("Item: %v", err)
	}
}

func TestAppendNilItemErrors(t *testing.T) {
	w := NewWorld()
	if err := w.Append("drops.list", "d-1", nil); err == nil {
		t.Fatal("expected error for nil append item")
	}
}
