package ui_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"net/url"
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
		`<span data-tc-field-error="n" aria-live="polite"></span>`,
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
		`<span data-tc-field-error="n" aria-live="polite">must be a whole number</span>`,
		`<span data-tc-form-error aria-live="polite">nothing to add</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if renderForm(t, ui.Failed(r, nil)) != renderForm(t) {
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
