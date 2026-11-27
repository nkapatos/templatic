package live

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a-h/templ"
)

type row struct {
	ID, Text string
}

func rowBody(r row) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<td>"+templ.EscapeString(r.Text)+"</td>")
		return err
	})
}

func rows(w *World, topic string) *Collection[row] {
	return NewCollection(w, topic, func(r row) string { return r.ID }).Renders(rowBody)
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func keysOf[T any](c *Collection[T], key func(T) string) string {
	var out []string
	for _, it := range c.Items() {
		out = append(out, key(it))
	}
	return strings.Join(out, ",")
}

// opsFrame is the decoded body of an ops or reset frame.
type opsFrame struct {
	Kind string `json:"kind"`
	Ver  uint64 `json:"ver"`
	HTML string `json:"html"`
	Ops  []struct {
		Op   string `json:"op"`
		K    string `json:"k"`
		HTML string `json:"html"`
	} `json:"ops"`
}

func decodeFrame(t *testing.T, frame string) opsFrame {
	t.Helper()
	_, data, ok := strings.Cut(frame, "data: ")
	if !ok {
		t.Fatalf("no data in frame %q", frame)
	}
	var f opsFrame
	if err := json.Unmarshal([]byte(data), &f); err != nil {
		t.Fatalf("frame %q: %v", data, err)
	}
	return f
}

// nextCollectionFrame skips cell frames and returns the next ops or reset frame for topic.
func nextCollectionFrame(t *testing.T, st *stream, topic string) (opsFrame, bool) {
	t.Helper()
	for {
		f, ok := st.next(t, time.Second)
		if !ok || f == "" {
			return opsFrame{}, false
		}
		if strings.Contains(f, `"topic":"`+topic+`"`) && (strings.Contains(f, `"kind":"ops"`) || strings.Contains(f, `"kind":"reset"`)) {
			return decodeFrame(t, f), true
		}
	}
}

func TestCollectionRendersContainerItemsAndVersion(t *testing.T) {
	w := NewWorld()
	c := rows(w, "orders").Elements("tbody", "tr").Limit(50).ItemAttrs(func(r row) templ.Attributes {
		return templ.Attributes{"class": "row-" + r.ID, "data-tc-item": "forged", "onclick": "alert(1)"}
	})
	if err := c.Upsert(row{"a", "A"}, row{"b", "<b>"}); err != nil {
		t.Fatal(err)
	}
	got := render(t, c)
	want := `<tbody data-tc-collection="orders" data-tc-ver="2" data-tc-limit="50">` +
		`<tr data-tc-item="b" class="row-b" onclick="` + templ.FailedSanitizationJS + `"><td>&lt;b&gt;</td></tr>` +
		`<tr data-tc-item="a" class="row-a" onclick="` + templ.FailedSanitizationJS + `"><td>A</td></tr></tbody>`
	if got != want {
		t.Fatalf("render:\n got %s\nwant %s", got, want)
	}
}

func TestCollectionOrderUpdateRemoveAndLimit(t *testing.T) {
	w := NewWorld()
	key := func(r row) string { return r.ID }
	head := rows(w, "head").Limit(3)
	tail := rows(w, "tail").Tail().Limit(3)
	for _, c := range []*Collection[row]{head, tail} {
		for _, id := range []string{"1", "2", "3", "4"} {
			if err := c.Upsert(row{ID: id}); err != nil {
				t.Fatal(err)
			}
		}
		if err := c.Upsert(row{"3", "updated"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := keysOf(head, key); got != "4,3,2" {
		t.Fatalf("head order %s", got)
	}
	if got := keysOf(tail, key); got != "2,3,4" {
		t.Fatalf("tail order %s", got)
	}
	if r, _ := head.Get("3"); r.Text != "updated" {
		t.Fatalf("update in place lost: %+v", r)
	}
	if _, ok := head.Get("1"); ok {
		t.Fatal("the limit must drop the oldest item")
	}
	before := head.Version()
	if err := head.Remove("3", "missing"); err != nil {
		t.Fatal(err)
	}
	if got, v := keysOf(head, key), head.Version(); got != "4,2" || v != before+1 {
		t.Fatalf("remove: order %s, version %d (was %d); a missing key must publish nothing", got, v, before)
	}
}

func TestCollectionRefusesBadKeysWithoutPublishing(t *testing.T) {
	w := NewWorld()
	c := rows(w, "keys")
	if err := c.Upsert(row{ID: "ok"}, row{ID: "bad key"}); err == nil {
		t.Fatal("a key outside the topic grammar must be an error")
	}
	if c.Len() != 0 || c.Version() != 0 {
		t.Fatalf("a refused batch must change nothing: len %d version %d", c.Len(), c.Version())
	}
	if err := c.Reset([]row{{ID: "a"}, {ID: "a"}}); err == nil {
		t.Fatal("a duplicate key in Reset must be an error")
	}
}

func TestCollectionResetIsOneVersionInDisplayOrder(t *testing.T) {
	w := NewWorld()
	c := rows(w, "reset")
	_ = c.Upsert(row{ID: "old"})
	if err := c.Reset([]row{{ID: "x"}, {ID: "y"}, {ID: "z"}}); err != nil {
		t.Fatal(err)
	}
	if got := keysOf(c, func(r row) string { return r.ID }); got != "x,y,z" || c.Version() != 2 {
		t.Fatalf("reset: %s at version %d", got, c.Version())
	}
	_ = c.Upsert(row{ID: "w"})
	if got := keysOf(c, func(r row) string { return r.ID }); got != "w,x,y,z" {
		t.Fatalf("a head insert after reset lands on top: %s", got)
	}
}

func TestCollectionReachesOnlySubscribedPages(t *testing.T) {
	w := NewWorld()
	c := rows(w, "feed")
	ts := serveWorld(t, w)
	shows := openStreamQuery(t, ts, "shows", "c=feed:0", nil)
	shows.discardUntilReady(t)
	other := openStream(t, ts, "other")
	other.discardUntilReady(t)

	if err := c.Upsert(row{"a", "A"}); err != nil {
		t.Fatal(err)
	}
	f, ok := nextCollectionFrame(t, shows, "feed")
	if !ok || f.Kind != "ops" || f.Ver != 1 || len(f.Ops) != 1 || f.Ops[0].K != "a" || !strings.Contains(f.Ops[0].HTML, `<div data-tc-item="a"><td>A</td></div>`) {
		t.Fatalf("subscribed page: %+v ok=%v", f, ok)
	}
	if _, ok := nextCollectionFrame(t, other, "feed"); ok {
		t.Fatal("a page that does not show the collection must not receive it")
	}
}

func TestCollectionCatchUpFromTheVersionThePageHolds(t *testing.T) {
	w := NewWorld()
	c := rows(w, "log").History(1 << 20)
	small := rows(w, "tiny").History(1)
	for i := range 5 {
		_ = c.Upsert(row{ID: fmt.Sprint(i)})
		_ = small.Upsert(row{ID: fmt.Sprint(i)})
	}
	ts := serveWorld(t, w)

	cases := []struct {
		name, query string
		topic       string
		kind        string
		keys        string
	}{
		{"within history: the ops after it", "c=log:3", "log", "ops", "3,4"},
		{"current: nothing", "c=log:5", "log", "", ""},
		{"past history: the whole set", "c=tiny:1", "tiny", "reset", ""},
		{"no version: the whole set", "c=log", "log", "reset", ""},
		{"ahead of the server: the whole set", "c=log:99", "log", "reset", ""},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openStreamQuery(t, ts, fmt.Sprintf("cu%d", i), tc.query, nil)
			st.discardUntilReady(t)
			f, ok := nextCollectionFrame(t, st, tc.topic)
			if tc.kind == "" {
				if ok {
					t.Fatalf("a page at the current version got %+v", f)
				}
				return
			}
			if !ok || f.Kind != tc.kind || f.Ver != 5 {
				t.Fatalf("got %+v ok=%v", f, ok)
			}
			if tc.kind == "ops" {
				var keys []string
				for _, op := range f.Ops {
					keys = append(keys, op.K)
				}
				if strings.Join(keys, ",") != tc.keys {
					t.Fatalf("catch-up ops %v", keys)
				}
			} else if strings.Count(f.HTML, "data-tc-item=") != 5 {
				t.Fatalf("reset must hold every item: %s", f.HTML)
			}
		})
	}
}

func TestCollectionSubscribeThroughCommands(t *testing.T) {
	w := NewWorld()
	c := rows(w, "late")
	_ = c.Upsert(row{ID: "a"}, row{ID: "b"})
	ts := serveWorld(t, w)
	st := openStream(t, ts, "late-sub")
	st.discardUntilReady(t)

	body := `{"sid":"late-sub","subscribe":{"late":1}}`
	resp, err := http.Post(ts.URL+"/live/commands", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("subscribe status %d", resp.StatusCode)
	}
	if f, ok := nextCollectionFrame(t, st, "late"); !ok || f.Kind != "ops" || f.Ver != 2 || len(f.Ops) != 1 || f.Ops[0].K != "b" {
		t.Fatalf("catch-up after a mid-stream subscribe: %+v ok=%v", f, ok)
	}
	_ = c.Upsert(row{ID: "c"})
	if f, ok := nextCollectionFrame(t, st, "late"); !ok || f.Ver != 3 {
		t.Fatalf("live ops after subscribing: %+v ok=%v", f, ok)
	}
}

func TestCollectionOpsArriveContiguousUnderConcurrentWriters(t *testing.T) {
	w := NewWorld()
	c := rows(w, "race")
	ts := serveWorld(t, w)
	st := openStreamQuery(t, ts, "race-sub", "c=race:0", nil)
	st.discardUntilReady(t)

	const writers, each = 8, 25
	var wg sync.WaitGroup
	for g := range writers {
		wg.Go(func() {
			for i := range each {
				_ = c.Upsert(row{ID: fmt.Sprintf("g%d-%d", g, i)})
			}
		})
	}
	wg.Wait()
	var held uint64
	for held < writers*each {
		f, ok := nextCollectionFrame(t, st, "race")
		if !ok {
			t.Fatalf("stream stopped at version %d of %d", held, writers*each)
		}
		if first := f.Ver - uint64(len(f.Ops)) + 1; first != held+1 {
			t.Fatalf("batch %d..%d after version %d: ops out of order or lost", first, f.Ver, held)
		}
		held = f.Ver
	}
}

func TestCollectionRetiredSessionIsUnsubscribed(t *testing.T) {
	w := NewWorld(WithResumeGrace(0))
	c := rows(w, "gone")
	ts := serveWorld(t, w)
	st := openStreamQuery(t, ts, "leaver", "c=gone:0", nil)
	st.discardUntilReady(t)
	drop(t, w, "leaver", st)
	waitFor(t, "retire", func() bool { return sessionOf(w, "leaver") == nil })
	c.mu.Lock()
	n := len(c.subs)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("a retired session must leave the collection's subscribers, %d left", n)
	}
}

func TestCollectionHistoryKeepsItsBudget(t *testing.T) {
	w := NewWorld()
	c := rows(w, "budget").History(4 << 10)
	for i := range 2000 {
		_ = c.Upsert(row{ID: fmt.Sprint(i), Text: strings.Repeat("x", 40)})
	}
	c.mu.Lock()
	bytes, n, first := c.ring.bytes, c.ring.n, c.ring.first
	c.mu.Unlock()
	if bytes > 4<<10 || n == 0 || first+uint64(n)-1 != c.Version() {
		t.Fatalf("ring holds %d ops, %d bytes, from version %d; collection at %d", n, bytes, first, c.Version())
	}
	ops, ok := c.ring.since(c.Version() - 3)
	if !ok || len(ops) != 3 {
		t.Fatalf("since the last three: %d ops ok=%v", len(ops), ok)
	}
	if _, ok := c.ring.since(first - 2); ok {
		t.Fatal("since a version older than the ring must miss")
	}
}
