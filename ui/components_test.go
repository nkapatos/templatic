package ui_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"html"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"
)

func ctxComp(markup string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, markup)
		return err
	})
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestCellPaintMatchesWireRenderRule(t *testing.T) {
	w := live.NewWorld()
	type inner struct{ A int }
	paint := func(name, want string, c templ.Component) {
		t.Helper()
		got := render(t, c)
		start := strings.Index(got, `>`) + 1
		end := strings.LastIndex(got, `<`)
		if text := html.UnescapeString(got[start:end]); text != want {
			t.Fatalf("cell %s painted %q, want %q (markup %q)", name, text, want, got)
		}
	}
	paint("nil", "", ui.Cell(live.NewCell(w, "t.nil", any(nil))))
	paint("string", "hi", ui.Cell(live.NewCell(w, "t.str", "hi")))
	paint("markup", "<b>", ui.Cell(live.NewCell(w, "t.mark", "<b>")))
	paint("number", "1.5", ui.Cell(live.NewCell(w, "t.num", 1.5)))
	paint("bool", "true", ui.Cell(live.NewCell(w, "t.bool", true)))
	paint("object", `{"a":1}`, ui.Cell(live.NewCell(w, "t.obj", map[string]int{"a": 1})))
	paint("array", `[1,2]`, ui.Cell(live.NewCell(w, "t.arr", []int{1, 2})))
	paint("struct", `{"A":1}`, ui.Cell(live.NewCell(w, "t.struct", inner{A: 1})))
}

func TestCellEmitsMarkerWithEscapedValue(t *testing.T) {
	w := live.NewWorld()
	tick := live.NewCell(w, "tick", "a<b")

	got := render(t, ui.Cell(tick))
	if got != `<span data-tc-cell="tick">a&lt;b</span>` {
		t.Fatalf("unexpected cell markup: %q", got)
	}
}

func TestSessionCellEmitsMarkerWithEscapedValue(t *testing.T) {
	got := render(t, ui.SessionCell("assistant.count", "a<b"))
	if got != `<span data-tc-cell="assistant.count">a&lt;b</span>` {
		t.Fatalf("unexpected session cell markup: %q", got)
	}
}

func TestSessionCellPaintMatchesCellPaint(t *testing.T) {
	w := live.NewWorld()
	type inner struct{ A int }
	values := map[string]any{
		"nil": nil, "empty": "", "string": "hi", "markup": "<b>",
		"number": 1.5, "int": 7, "bool": true,
		"object": map[string]int{"a": 1}, "array": []int{1, 2}, "struct": inner{A: 1},
	}
	for name, v := range values {
		topic := "t." + name
		want := render(t, ui.Cell(live.NewCell(w, topic, v)))
		if got := render(t, ui.SessionCell(topic, v)); got != want {
			t.Fatalf("session cell %s painted %q, world cell %q", name, got, want)
		}
	}
}

func TestSessionCellRejectsBadTopic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for an invalid topic")
		}
	}()
	ui.SessionCell("bad topic", "")
}

func TestIslandEmitsTagPropsAndSSRContent(t *testing.T) {
	type Props struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	ssr := ctxComp("<p>server markup</p>")

	got := render(t, ui.Island("user-card", Props{Name: "ada", N: 3}, ssr))

	if !strings.Contains(got, `<user-card data-tc-island="user-card"><script type="application/tc-props" data-tc-for="user-card">`) {
		t.Fatalf("island wrapper wrong: %q", got)
	}
	if !strings.Contains(got, `"name":"ada"`) || !strings.Contains(got, `"n":3`) {
		t.Fatalf("props payload wrong: %q", got)
	}
	if !strings.Contains(got, "<p>server markup</p></user-card>") {
		t.Fatalf("SSR content not wrapped: %q", got)
	}
}

func TestIslandRejectsBadTags(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	render(t, ui.Island("nope", nil, nil))
}

func TestActionEmitsTriggerWithEscapedArgs(t *testing.T) {
	type Args struct {
		ID string `json:"id"`
	}
	got := render(t, ui.Action("cart.add", Args{ID: `x"1`}))
	if !strings.Contains(got, `<button type="button" data-tc-action="cart.add" data-tc-args="`) {
		t.Fatalf("action wrapper wrong: %q", got)
	}
	var payload struct {
		ID string `json:"id"`
	}
	start := strings.Index(got, `data-tc-args="`) + len(`data-tc-args="`)
	end := strings.Index(got[start:], `"`) + start
	if err := json.Unmarshal([]byte(html.UnescapeString(got[start:end])), &payload); err != nil {
		t.Fatalf("args payload not valid JSON: %q", got)
	}
	if payload.ID != `x"1` {
		t.Fatalf("args payload wrong: %q", payload.ID)
	}
	if !strings.HasSuffix(got, "</button>") {
		t.Fatalf("button not closed: %q", got)
	}
}

func TestActionRendersChildrenViaContext(t *testing.T) {
	comp := ctxComp("<b>+</b>")
	ctx := templ.WithChildren(context.Background(), comp)
	var buf bytes.Buffer
	if err := ui.Action("inc", nil).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<b>+</b>") {
		t.Fatalf("children missing: %q", buf.String())
	}
}

func TestRegionEmitsMarkerWithPageContent(t *testing.T) {
	got := render(t, ui.Region("workspace.surface", ctxComp("<p>initial</p>")))
	if got != `<div data-tc-region="workspace.surface"><p>initial</p></div>` {
		t.Fatalf("region markup wrong: %q", got)
	}
	if render(t, ui.Region("empty", nil)) != `<div data-tc-region="empty"></div>` {
		t.Fatalf("nil-content region markup wrong")
	}
}

func TestRegionRejectsBadTopic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on malformed topic")
		}
	}()
	_ = ui.Region("bad topic!", nil)
}

func TestActionLinkRejectsUnsafeHref(t *testing.T) {
	got := render(t, ui.ActionLink("note.open", nil, `/x" onmouseover=alert(1) x="`))
	start := strings.Index(got, `href="`) + len(`href="`)
	end := strings.Index(got[start:], `"`) + start
	if want := `/x" onmouseover=alert(1) x="`; html.UnescapeString(got[start:end]) != want {
		t.Fatalf("href round trip wrong: %q", html.UnescapeString(got[start:end]))
	}

	got = render(t, ui.ActionLink("note.open", nil, "javascript:alert(1)"))
	if strings.Contains(got, `href="javascript:`) {
		t.Fatalf("javascript: href must be sanitized: %q", got)
	}
	if !strings.Contains(got, `about:invalid`) { // templ's sentinel
		t.Fatalf("unsafe scheme must fall to the inert sentinel: %q", got)
	}
}

func TestActionLinkEmitsAnchorWithFallback(t *testing.T) {
	type Args struct {
		ID string `json:"id"`
	}
	got := render(t, ui.ActionLink("note.open", Args{ID: `x"1`}, "/workspace/x?1"))
	if !strings.Contains(got, `<a href="/workspace/x?1" data-tc-action="note.open" data-tc-args="`) {
		t.Fatalf("action link wrapper wrong: %q", got)
	}
	if !strings.HasSuffix(got, "</a>") {
		t.Fatalf("anchor not closed: %q", got)
	}
	var payload struct {
		ID string `json:"id"`
	}
	start := strings.Index(got, `data-tc-args="`) + len(`data-tc-args="`)
	end := strings.Index(got[start:], `"`) + start
	if err := json.Unmarshal([]byte(html.UnescapeString(got[start:end])), &payload); err != nil {
		t.Fatalf("args payload not valid JSON: %q", got)
	}
	if payload.ID != `x"1` {
		t.Fatalf("args payload wrong: %q", payload.ID)
	}
}

func TestActionLinkRendersChildrenViaContext(t *testing.T) {
	comp := ctxComp("<span>Roadmap</span>")
	ctx := templ.WithChildren(context.Background(), comp)
	var buf bytes.Buffer
	if err := ui.ActionLink("note.open", nil, "/workspace/roadmap").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<span>Roadmap</span></a>") {
		t.Fatalf("children missing: %q", buf.String())
	}
}

func TestFormEmitsFormWithFallbackAction(t *testing.T) {
	comp := ctxComp("<input name=\"title\">")
	ctx := templ.WithChildren(context.Background(), comp)
	var buf bytes.Buffer
	if err := ui.Form("note.capture", "/quick-capture?a=1&b=2").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, `<form method="POST" action="/quick-capture?a=1&amp;b=2" data-tc-form="note.capture">`) {
		t.Fatalf("form wrapper wrong: %q", got)
	}
	if !strings.HasSuffix(got, `</form>`) || !strings.Contains(got, `<input name="title">`) {
		t.Fatalf("form children wrong: %q", got)
	}
}

func TestFormMultipart(t *testing.T) {
	var buf bytes.Buffer
	if err := ui.Form("import.upload", "/import", ui.Multipart()).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	want := `<form method="POST" action="/import" data-tc-form="import.upload" enctype="multipart/form-data" data-tc-multipart></form>`
	if buf.String() != want {
		t.Fatalf("got %q", buf.String())
	}
}

func TestFormRejectsBadNames(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on malformed name")
		}
	}()
	_ = ui.Form("bad name!", "/x")
}

func TestFormRequiresFallbackAction(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on empty fallback action")
		}
	}()
	_ = ui.Form("ok.name", "")
}

func TestRegionRendersAsTemplComponent(t *testing.T) {
	w := live.NewWorld()
	feed := w.Region("feed", func() templ.Component {
		return ctxComp("<li>ok</li>")
	})
	got := render(t, feed)
	if got != `<div data-tc-region="feed"><li>ok</li></div>` {
		t.Fatalf("region markup wrong: %q", got)
	}
}

func TestStreamEmitsContainer(t *testing.T) {
	got := render(t, ui.Stream("drops.list", ctxComp(`<div data-tc-item="d-1">ok</div>`)))
	if got != `<div data-tc-stream="drops.list"><div data-tc-item="d-1">ok</div></div>` {
		t.Fatalf("stream container wrong: %q", got)
	}
}

func TestStreamTailOptionEmitsDirection(t *testing.T) {
	got := render(t, ui.Stream("brain.conv.a1", ctxComp(`<div data-tc-item="a1-1">hi</div>`), ui.Tail()))
	if got != `<div data-tc-stream="brain.conv.a1" data-tc-dir="tail"><div data-tc-item="a1-1">hi</div></div>` {
		t.Fatalf("tail stream container wrong: %q", got)
	}
	plain := render(t, ui.Stream("drops.list", nil))
	if plain != `<div data-tc-stream="drops.list"></div>` {
		t.Fatalf("default stream container must not carry a direction: %q", plain)
	}
}

func TestStreamItemMatchesAppendWrapper(t *testing.T) {
	got := render(t, ui.StreamItem("d-1", ctxComp("<p>ok</p>")))
	if got != `<div data-tc-item="d-1"><p>ok</p></div>` {
		t.Fatalf("stream item wrapper wrong: %q", got)
	}
}

func TestStreamRejectBadNames(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on malformed stream topic")
		}
	}()
	_ = ui.Stream("bad name!", nil)
}

func TestStreamItemRejectsBadKey(t *testing.T) {
	var buf bytes.Buffer
	if err := ui.StreamItem("bad key!", nil).Render(context.Background(), &buf); err == nil || buf.Len() != 0 {
		t.Fatalf("err %v, wrote %q", err, buf.String())
	}
}

func TestIslandKeepsScriptEndInsidePropsTag(t *testing.T) {
	got := render(t, ui.Island("x-chart", map[string]string{"t": `</script><script>alert(1)</script>`}, nil))
	if strings.Contains(got, "<script>alert(1)") || strings.Count(got, "</script>") != 1 {
		t.Fatalf("props closed the script tag: %q", got)
	}
	if !strings.Contains(got, `</script>`) {
		t.Fatalf("props not escaped: %q", got)
	}
}

func TestIslandRejectsTagNotStartingWithLetter(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for a tag starting with a digit")
		}
	}()
	ui.Island("1-chart", nil, nil)
}
