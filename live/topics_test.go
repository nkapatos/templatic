package live

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
)

// framesUntilReady reads frames until ready and returns them.
func framesUntilReady(t *testing.T, st *stream) string {
	t.Helper()
	var b strings.Builder
	for {
		f, ok := st.next(t, 2*time.Second)
		if !ok || f == "" {
			t.Fatalf("ready never arrived: %s", b.String())
		}
		b.WriteString(f + "\n")
		if strings.Contains(f, `"kind":"ready"`) {
			return b.String()
		}
	}
}

func TestPageGetsOnlyTheCellsItUses(t *testing.T) {
	w := NewWorld()
	shown, other := NewCell(w, "shown", 1), NewCell(w, "other", 1)
	ts := serveStrict(t, w)
	r := openStreamQuery(t, ts, "s1", "c=shown", nil)
	defer r.Close()
	if got := framesUntilReady(t, r); !strings.Contains(got, `"topic":"shown"`) || strings.Contains(got, `"topic":"other"`) {
		t.Fatalf("initial frames: %s", got)
	}
	other.Set(2)
	shown.Set(3)
	var got []string
	for {
		f, ok := r.next(t, 300*time.Millisecond)
		if !ok {
			break
		}
		got = append(got, f)
	}
	if len(got) != 1 || !strings.Contains(got[0], `"topic":"shown"`) || !strings.Contains(got[0], `"v":3`) {
		t.Fatalf("got %q, want only shown's update", got)
	}
}

func TestSubscribingLaterSendsTheCellsValueThenItsUpdates(t *testing.T) {
	w := NewWorld()
	late := NewCell(w, "late", 7)
	ts := serveStrict(t, w)
	r := openStreamQuery(t, ts, "s1", "", nil)
	defer r.Close()
	framesUntilReady(t, r)

	resp, err := http.Post(ts.URL+"/live/commands", "application/json", bytes.NewBufferString(`{"sid":"s1","subscribe":{"late":-1}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if f, _ := r.next(t, 2*time.Second); !strings.Contains(f, `"v":7`) {
		t.Fatalf("subscribe did not send the current value: %q", f)
	}
	late.Set(8)
	if f, _ := r.next(t, 2*time.Second); !strings.Contains(f, `"v":8`) {
		t.Fatalf("got %q", f)
	}
}

func TestRefreshSkipsPagesWithoutTheRegion(t *testing.T) {
	w := NewWorld()
	region := DeclareSessionRegion[string]("panel").Renders(func(m string) templ.Component { return text(m) })
	ts := serveStrict(t, w)
	with := openStreamQuery(t, ts, "with", "c=panel", nil)
	without := openStreamQuery(t, ts, "without", "", nil)
	framesUntilReady(t, with)
	framesUntilReady(t, without)

	var asked []string
	if err := region.Refresh(w, func(s *Session) (string, bool) {
		asked = append(asked, s.SID())
		return "m", true
	}); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "with" {
		t.Fatalf("model read for %v, want only the page with the region", asked)
	}
	if f, _ := with.next(t, 2*time.Second); !strings.Contains(f, `"html":"m"`) {
		t.Fatalf("got %q", f)
	}
}
