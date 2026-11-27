package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/live"
)

func renderString(t *testing.T, ctx context.Context, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

type openArgs struct {
	ID string `json:"id"`
}

func TestLinkInheritsDeclaredSwaps(t *testing.T) {
	open := live.NewCommand[openArgs]("note.open").Swaps("workspace.surface")
	got := renderString(t, context.Background(),
		Link(open, openArgs{ID: "n1"}, "/workspace/n1", Class("nav", "active")))
	for _, want := range []string{`data-tc-action="note.open"`, `data-tc-swaps="workspace.surface"`, `class="nav active"`, `&#34;n1&#34;`} {
		if !strings.Contains(got, want) && !strings.Contains(got, strings.ReplaceAll(want, "&#34;", "&quot;")) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	btn := renderString(t, context.Background(), Button(open, openArgs{ID: "n1"}, Attrs(templ.Attributes{"title": `a"b`, "disabled": true})))
	if !strings.Contains(btn, ` disabled title="a&#34;b"`) {
		t.Fatalf("attrs: %s", btn)
	}
}

func TestAttrsRefusesFrameworkMarkers(t *testing.T) {
	for _, k := range []string{"data-tc-action", "href", "class"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("Attrs(%s) must panic", k)
				}
			}()
			Attrs(templ.Attributes{k: "x"})
		}()
	}
}

func TestAttrsTakesAHook(t *testing.T) {
	cmd := live.NewCommand[openArgs]("hooked.form")
	got := renderString(t, context.Background(), FormFor(cmd, "/x", Attrs(Hook("reset", map[string]int{"n": 1}))))
	if !strings.Contains(got, `data-tc-hook="reset"`) || !strings.Contains(got, `data-tc-hook-args="{&#34;n&#34;:1}"`) {
		t.Fatalf("hook attributes missing: %s", got)
	}
}

func TestStyleHooksOnContainers(t *testing.T) {
	ctx := context.Background()
	if got := renderString(t, ctx, Region("r", nil, Class("pane"))); got != `<div data-tc-region="r" class="pane"></div>` {
		t.Fatal(got)
	}
}

func TestBindDeclaredSessionCell(t *testing.T) {
	count := live.DeclareSessionCell("assistant.count", "")
	if got := renderString(t, context.Background(), Bind(count, "3")); got != `<span data-tc-cell="assistant.count">3</span>` {
		t.Fatal(got)
	}
	if a := BindAttr(count, "data-n", "3"); a["data-tc-a-data-n"] != "assistant.count" {
		t.Fatal(a)
	}
}
