package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	templruntime "github.com/a-h/templ/runtime"
)

func TestHookAttributesEscape(t *testing.T) {
	attrs := Hook("tooltip", map[string]string{"text": `"><script>x</script>`})
	if attrs["data-tc-hook"] != "tooltip" {
		t.Fatalf("name: %v", attrs)
	}
	var buf bytes.Buffer
	if err := templruntime.RenderAttributes(context.Background(), &buf, &bytes.Buffer{}, "div", attrs); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "<script>") || strings.Contains(out, `"><`) {
		t.Fatalf("args not attribute-escaped: %s", out)
	}
	if !strings.Contains(out, `data-tc-hook-args="{&#34;text&#34;:`) {
		t.Fatalf("args missing: %s", out)
	}
	if _, ok := Hook("autosize", nil)["data-tc-hook-args"]; ok {
		t.Fatal("nil args must omit data-tc-hook-args")
	}
}

func TestHookNameGrammar(t *testing.T) {
	for _, bad := range []string{"", "has space", `q"uote`, "a/b"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Hook(%q) did not panic", bad)
				}
			}()
			Hook(bad, nil)
		}()
	}
}
