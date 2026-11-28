package live

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

type tagIn struct {
	ID   string   `json:"id"`
	Tags []string `json:"tag"`
}

func (in tagIn) Validate() error {
	if in.ID == "" {
		return Invalid("id", "required")
	}
	return nil
}

func formReq(v url.Values) *http.Request {
	r := httptest.NewRequest("POST", "/x", strings.NewReader(v.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func TestCommandDecodeMirrorsJSPath(t *testing.T) {
	cmd := NewCommand[tagIn]("note.tag")
	in, err := cmd.Decode(formReq(url.Values{"id": {"n1"}, "tag": {"a"}}))
	if err != nil || in.ID != "n1" || len(in.Tags) != 1 || in.Tags[0] != "a" {
		t.Fatalf("single: %+v %v", in, err)
	}
	in, err = cmd.Decode(formReq(url.Values{"id": {"n1"}, "tag": {"a", "b"}}))
	if err != nil || len(in.Tags) != 2 {
		t.Fatalf("repeated: %+v %v", in, err)
	}
	_, err = cmd.Decode(formReq(url.Values{"tag": {"a"}}))
	var ie *InputError
	if !errors.As(err, &ie) || ie.Field != "id" {
		t.Fatalf("validate must run: %v", err)
	}
	var viaJS tagIn
	_ = formArgs(url.Values{"id": {"n1"}, "tag": {"a"}}).Into(&viaJS)
	if viaJS.ID != "n1" || len(viaJS.Tags) != 1 {
		t.Fatalf("js path: %+v", viaJS)
	}
}

func TestCommandHandleRegistersTypedHandler(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "c1")
	cmd := NewCommand[tagIn]("note.tag").Swaps("workspace.surface")
	if cmd.SwapTopic() != "workspace.surface" || cmd.Name() != "note.tag" {
		t.Fatal("declaration lost")
	}
	var got tagIn
	cmd.Handle(w, func(_ *Session, in tagIn) (any, error) { got = in; return nil, nil })
	if _, err := w.commands["note.tag"](s, jsonArgs(jsontext.Value(`{"id":"x","tag":["p","q"]}`))); err != nil {
		t.Fatal(err)
	}
	if got.ID != "x" || len(got.Tags) != 2 {
		t.Fatalf("%+v", got)
	}
	if _, err := w.commands["note.tag"](s, jsonArgs(jsontext.Value(`{}`))); err == nil {
		t.Fatal("Validate must refuse")
	}
}

func TestDeclaredSessionCellAndValue(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "d1")
	q := DeclareSessionCell("search.q", "init")
	if c := q.For(s); c.Get() != "init" || c != q.For(s) {
		t.Fatal("For must declare once with the initial")
	}
	q.For(s).Set("go")
	if SessionCell(w, s, "search.q", "").Get() != "go" {
		t.Fatal("decl and raw SessionCell must share the cell")
	}
	s2 := newTestSession(t, w, "d2")
	s2.cells = map[string]cellView{"search.q": &seedCell{raw: jsontext.Value(`"seeded"`)}}
	if q.For(s2).Get() != "seeded" {
		t.Fatal("seed not adopted")
	}

	recent := DeclareSessionValue("search.recent", func() []string { return []string{} })
	recent.For(s).Update(func(o []string) []string { return append(o, "a") })
	if len(recent.For(s2).Get()) != 0 || len(recent.For(s).Get()) != 1 {
		t.Fatal("values must be per session, initial not shared")
	}
}

func TestSessionRegionRenderAndSwap(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "r1")
	r := DeclareSessionRegion[string]("daily.surface")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("unbound region must panic")
			}
		}()
		_ = r.Swap(s, "x")
	}()
	r.Renders(func(m string) templ.Component {
		return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
			_, err := io.WriteString(w, "<p>"+m+"</p>")
			return err
		})
	})
	var sb strings.Builder
	if err := r.Render("first").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if sb.String() != `<div data-tc-region="daily.surface"><p>first</p></div>` {
		t.Fatalf("render: %s", sb.String())
	}
	if err := r.Swap(s, "next"); err != nil {
		t.Fatal(err)
	}
	ev := <-s.conn.inbox
	if ev.kind != KindSwap || ev.topic != "daily.surface" || !strings.Contains(string(ev.body), "next") {
		t.Fatalf("swap event: %+v", ev)
	}
}

func TestCellFamily(t *testing.T) {
	w := NewWorld()
	f := NewCellFamilyFunc(w, "brain.status", func(k string) string { return "idle:" + k })
	a := f.Get("c1")
	if a != f.Get("c1") || a.Get() != "idle:c1" || a.Name() != "brain.status.c1" {
		t.Fatal("Get must declare once under prefix.key")
	}
	if _, ok := w.cells["brain.status.c1"]; !ok {
		t.Fatal("family cells are world cells")
	}
	f.Forget("c1")
	if f.Has("c1") {
		t.Fatal("Forget kept the key")
	}
	if _, ok := w.cells["brain.status.c1"]; ok {
		t.Fatal("Forget must drop the world cell")
	}
	again := f.Get("c1")
	again.Set("x")
	if again == a || again.Get() != "x" {
		t.Fatal("a forgotten key re-declares fresh")
	}
	again.Set("y")
	again.Set("z")
	f.Forget("c1")
	if f.Get("c1").Version() < 3 {
		t.Fatal("a re-declared cell must start above the forgotten versions")
	}
	f.Get("c2")
	f.Get("c3")
	f.Clear()
	if len(w.cells) != 0 {
		t.Fatalf("Clear left %d cells", len(w.cells))
	}
}

func TestFormAndPathValues(t *testing.T) {
	type askIn struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	cmd := NewCommand[askIn]("brain.ask")
	var got askIn
	mux := http.NewServeMux()
	mux.Handle("POST /brain/{id}", cmd.Form(NewWorld(),
		func(_ *http.Request, _ *Session, in askIn) (string, error) {
			got = in
			if in.Text == "" {
				return "", Invalid("text", "empty")
			}
			if in.Text == "boom" {
				return "", errors.New("store down")
			}
			if in.Text == "gone" {
				return "", fmt.Errorf("lookup: %w", NotFound("no such conversation"))
			}
			return "/brain/" + in.ID, nil
		},
		func(w http.ResponseWriter, _ *http.Request, err error) {
			// every error comes here: the app renders its own page for each
			if ies := InputErrors(err); len(ies) > 0 {
				w.WriteHeader(422)
				_, _ = io.WriteString(w, ies[0].Field)
				return
			}
			var se *StatusError
			if errors.As(err, &se) {
				w.WriteHeader(se.Code)
				_, _ = io.WriteString(w, "app page: "+se.Message)
				return
			}
			w.WriteHeader(500)
			_, _ = io.WriteString(w, "app error page")
		}))
	do := func(form url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/brain/c1", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		mux.ServeHTTP(rec, r)
		return rec
	}
	rec := do(url.Values{"id": {"other"}, "text": {"hi"}})
	if rec.Code != 303 || rec.Header().Get("Location") != "/brain/c1" || got.ID != "c1" {
		t.Fatalf("path must win and redirect: %d %q %+v", rec.Code, rec.Header().Get("Location"), got)
	}
	if rec := do(url.Values{}); rec.Code != 422 || rec.Body.String() != "text" {
		t.Fatalf("invalid: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(url.Values{"text": {"boom"}}); rec.Code != 500 || rec.Body.String() != "app error page" {
		t.Fatalf("an internal error goes to invalid: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(url.Values{"text": {"gone"}}); rec.Code != 404 || rec.Body.String() != "app page: no such conversation" {
		t.Fatalf("a StatusError goes to invalid: %d %q", rec.Code, rec.Body.String())
	}
}

func TestDeclShortcuts(t *testing.T) {
	w := NewWorld()
	s := newTestSession(t, w, "x4")
	c := DeclareSessionCell("n", 1)
	c.Set(s, 2)
	c.Update(s, func(o int) int { return o + 1 })
	v := DeclareSessionValue("k", func() int { return 0 })
	v.Set(s, 5)
	if c.Get(s) != 3 || v.Get(s) != 5 {
		t.Fatal(c.Get(s), v.Get(s))
	}
}

func TestDecodePathValuesWithoutServeMux(t *testing.T) {
	type in struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	r := httptest.NewRequest("POST", "/clients/c42/status", strings.NewReader("id=forged&status=ok"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", "c42")
	got, err := DecodeForm[in](r)
	if err != nil || got.ID != "c42" || got.Status != "ok" {
		t.Fatalf("%+v %v", got, err)
	}
}
