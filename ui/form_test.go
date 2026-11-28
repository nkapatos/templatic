package ui_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"
)

type addIn struct {
	N     int    `json:"n"`
	Title string `json:"title"`
}

func (in addIn) Validate() error {
	var errs []error
	if in.N > 100 {
		errs = append(errs, live.Invalid("n", "at most 100"))
	}
	if in.Title == "" {
		errs = append(errs, live.Invalid("", "nothing to add"))
	}
	return errors.Join(errs...)
}

var add = live.NewCommand[addIn]("count.add")

var fields = templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
	if _, err := io.WriteString(w, `<input name="n" value="`+templ.EscapeString(ui.FieldValue(ctx, "n", "1"))+`">`); err != nil {
		return err
	}
	if err := ui.FieldError("n").Render(ctx, w); err != nil {
		return err
	}
	return ui.FormError().Render(ctx, w)
})

func renderForm(t *testing.T, opts ...ui.Option) string {
	t.Helper()
	var buf bytes.Buffer
	ctx := templ.WithChildren(context.Background(), fields)
	if err := ui.FormFor(add, "/add", opts...).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestFormFirstRenderHasEmptySlotsAndDefaults(t *testing.T) {
	got := renderForm(t)
	for _, want := range []string{
		`value="1"`,
		` data-tc-field-error="n" aria-live="polite"></span>`,
		`<span data-tc-form-error aria-live="polite"></span>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
}

func TestFailedRerendersValuesAndErrors(t *testing.T) {
	r := httptest.NewRequest("POST", "/add", strings.NewReader(url.Values{"n": {"<500>"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err := add.Decode(r)
	if err == nil {
		t.Fatal("decode must refuse")
	}
	got := renderForm(t, ui.Failed(r, err))
	for _, want := range []string{
		`value="&lt;500&gt;"`,
		` data-tc-field-error="n" aria-live="polite">must be a whole number</span>`,
		`<span data-tc-form-error aria-live="polite">nothing to add</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	// each render numbers its slots afresh; the rest must match
	formNo := regexp.MustCompile(`tc-f\d+-`)
	if formNo.ReplaceAllString(renderForm(t, ui.Failed(r, nil)), "") != formNo.ReplaceAllString(renderForm(t), "") {
		t.Fatal("Failed with a nil error must change nothing")
	}
}

func TestFormForRefusesNonFormInput(t *testing.T) {
	type nested struct {
		Addr struct{ City string } `json:"addr"`
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "not a form field") {
			t.Fatalf("panic = %v", r)
		}
	}()
	_ = ui.FormFor(live.NewCommand[nested]("x.nested"), "/x")
}

func TestFieldValuesKeepsTheSubmittedSelection(t *testing.T) {
	var got []string
	tags := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		got = ui.FieldValues(ctx, "tags", []string{"default"})
		return nil
	})
	render := func(opts ...ui.Option) []string {
		t.Helper()
		got = nil
		ctx := templ.WithChildren(context.Background(), tags)
		if err := ui.FormFor(add, "/add", opts...).Render(ctx, io.Discard); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if v := render(); strings.Join(v, ",") != "default" {
		t.Fatalf("first render: %v", v)
	}
	for body, want := range map[string]string{"n=x&tags=a&tags=c": "a,c", "n=x": ""} {
		r := httptest.NewRequest("POST", "/add", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		_, err := add.Decode(r)
		if v := render(ui.Failed(r, err)); strings.Join(v, ",") != want {
			t.Fatalf("%s: got %v, want %q", body, v, want)
		}
	}
}

func TestFieldAttrsMarkARefusedFieldLikeTheRuntime(t *testing.T) {
	r := httptest.NewRequest("POST", "/add", strings.NewReader(url.Values{"n": {"500"}, "title": {"x"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err := add.Decode(r)
	input := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		attrs := ui.FieldAttrs(ctx, "n")
		if _, err := io.WriteString(w, `<input name="n"`); err != nil {
			return err
		}
		for _, k := range slices.Sorted(maps.Keys(attrs)) {
			if _, err := io.WriteString(w, " "+k+`="`+templ.EscapeString(attrs[k].(string))+`"`); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, `>`); err != nil {
			return err
		}
		return ui.FieldError("n", ui.Class("invalid-feedback")).Render(ctx, w)
	})
	render := func(opts ...ui.Option) string {
		var buf bytes.Buffer
		if err := ui.FormFor(add, "/add", opts...).Render(templ.WithChildren(context.Background(), input), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	got := render(ui.Failed(r, err))
	m := regexp.MustCompile(`aria-describedby="([^"]+)"`).FindStringSubmatch(got)
	if !strings.Contains(got, `aria-invalid="true"`) || m == nil || !strings.Contains(got, `<span id="`+m[1]+`" data-tc-field-error="n"`) {
		t.Fatalf("the input must be marked and point at its slot: %s", got)
	}
	if !strings.Contains(got, `class="invalid-feedback">at most 100</span>`) {
		t.Fatalf("the slot takes its options: %s", got)
	}
	if plain := render(); strings.Contains(plain, "aria-invalid") || strings.Contains(plain, "aria-describedby") {
		t.Fatalf("an unrefused field is not marked: %s", plain)
	}
	slot := regexp.MustCompile(`<span id="[^"]+"`)
	if a, b := render(ui.Failed(r, err)), render(ui.Failed(r, err)); slot.FindString(a) == slot.FindString(b) {
		t.Fatal("two forms on a page must not share a slot id")
	}
}

func TestFormErrorTakesOptions(t *testing.T) {
	var buf bytes.Buffer
	if err := ui.FormError(ui.Class("alert"), ui.Attrs(templ.Attributes{"role": "alert"})).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != `<span data-tc-form-error aria-live="polite" class="alert" role="alert"></span>` {
		t.Fatalf("got %s", got)
	}
}

func TestFormForNamesTheRegionItsCommandSwaps(t *testing.T) {
	cmd := live.NewCommand[addIn]("count.add.swapping").Swaps("tally")
	var buf bytes.Buffer
	if err := ui.FormFor(cmd, "/add").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `data-tc-swaps="tally"`) {
		t.Fatalf("got %s", buf.String())
	}
}
