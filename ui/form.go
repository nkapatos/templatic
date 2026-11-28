package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/form"
	"github.com/nkapatos/templatic/internal/markup"
	"github.com/nkapatos/templatic/internal/pagestate"
	"github.com/nkapatos/templatic/live"
)

// Form renders a form that raises the command name on submit; without
// JavaScript the browser POSTs to action.
//
// Deprecated: use FormFor.
func Form(name string, action string, opts ...FormOption) templ.Component {
	if !markup.ValidName(name) {
		panic("templatic: form command must match [A-Za-z0-9_.-]+: " + name)
	}
	if action == "" {
		panic("templatic: form needs a no-JS fallback action URL")
	}
	cfg := buildConfig(opts)
	extra := ""
	if cfg.multipart {
		extra = ` enctype="multipart/form-data" data-tc-multipart`
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if cfg.failed != nil {
			ctx = context.WithValue(ctx, failedKey{}, cfg.failed)
		}
		ctx = context.WithValue(ctx, formKey{}, formSeq.Add(1))
		if _, err := fmt.Fprintf(w,
			`<form method="POST" action=%q data-tc-form=%q%s`,
			templ.EscapeString(action), name, extra); err != nil {
			return err
		}
		if err := cfg.writeExtras(w); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `>`); err != nil {
			return err
		}
		if ps := pagestate.From(ctx); ps != nil && ps.Tokens != nil {
			if id := ps.Tokens.SubmitID(); id != "" {
				if _, err := fmt.Fprintf(w, `<input type="hidden" name=%q value=%q>`,
					form.SubmitField, id); err != nil {
					return err
				}
			}
		}
		if children := templ.GetChildren(ctx); children != nil {
			if err := children.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</form>`)
		return err
	})
}

// FormOption configures a Form.
type FormOption = Option

// Multipart makes a FormFor form post multipart/form-data, for file fields.
func Multipart() FormOption { return func(c *elemConfig) { c.multipart = true } }

// FormFor renders a form for cmd that POSTs to action, where cmd.Form serves
// it with and without JavaScript. It panics when In has a field no form can
// fill.
func FormFor[In any](cmd *live.Command[In], action string, opts ...Option) templ.Component {
	if err := form.Check(reflect.TypeFor[In]()); err != nil {
		panic("templatic: ui.FormFor(" + cmd.Name() + "): " + err.Error())
	}
	return Form(cmd.Name(), action, opts...)
}

// failedForm is a refused submission: what was sent and why.
type failedForm struct {
	values  url.Values
	fields  map[string]string
	message string
}

type failedKey struct{}

// formKey holds the form render's number, which prefixes its error slots' ids so two forms never share one.
type formKey struct{}

var formSeq atomic.Uint64

// slotID is the id of field name's error slot in the form ctx renders, or "" outside a form.
func slotID(ctx context.Context, name string) string {
	n, ok := ctx.Value(formKey{}).(uint64)
	if !ok {
		return ""
	}
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	return "tc-f" + strconv.FormatUint(n, 10) + "-" + clean
}

// Failed re-renders a form after a refused no-JS POST with the errors from
// cmd.Decode; a nil err changes nothing.
func Failed(r *http.Request, err error) Option {
	ies := live.InputErrors(err)
	if len(ies) == 0 {
		return nil
	}
	f := &failedForm{fields: map[string]string{}}
	if r != nil {
		f.values = r.PostForm
	}
	var whole []string
	for _, ie := range ies {
		if ie.Field == "" {
			whole = append(whole, ie.Message)
		} else if _, dup := f.fields[ie.Field]; !dup {
			f.fields[ie.Field] = ie.Message
		}
	}
	f.message = strings.Join(whole, "; ")
	return func(c *elemConfig) { c.failed = f }
}

// FieldError renders the error slot of field name; place it inside the form. Class and Attrs style it.
func FieldError(name string, opts ...Option) templ.Component {
	cfg := buildConfig(opts)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		msg := ""
		if f, ok := ctx.Value(failedKey{}).(*failedForm); ok {
			msg = f.fields[name]
		}
		if _, err := io.WriteString(w, `<span`); err != nil {
			return err
		}
		if id := slotID(ctx, name); id != "" {
			if err := markup.WriteStrings(w, ` id="`, id, `"`); err != nil {
				return err
			}
		}
		if err := markup.WriteStrings(w, ` data-tc-field-error="`, templ.EscapeString(name), `" aria-live="polite"`); err != nil {
			return err
		}
		if err := cfg.writeExtras(w); err != nil {
			return err
		}
		return markup.WriteStrings(w, `>`, templ.EscapeString(msg), `</span>`)
	})
}

// FormError renders the error slot for errors that name no field. Class and Attrs style it.
func FormError(opts ...Option) templ.Component {
	cfg := buildConfig(opts)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		msg := ""
		if f, ok := ctx.Value(failedKey{}).(*failedForm); ok {
			msg = f.message
		}
		if _, err := io.WriteString(w, `<span data-tc-form-error aria-live="polite"`); err != nil {
			return err
		}
		if err := cfg.writeExtras(w); err != nil {
			return err
		}
		return markup.WriteStrings(w, `>`, templ.EscapeString(msg), `</span>`)
	})
}

// FieldAttrs returns the attributes field name's input takes after Failed refused it: aria-invalid and
// aria-describedby naming its FieldError, as the runtime sets them with JavaScript. Otherwise it is empty.
func FieldAttrs(ctx context.Context, name string) templ.Attributes {
	f, ok := ctx.Value(failedKey{}).(*failedForm)
	if !ok || f.fields[name] == "" {
		return templ.Attributes{}
	}
	attrs := templ.Attributes{"aria-invalid": "true"}
	if id := slotID(ctx, name); id != "" {
		// data-tc-described lets the runtime clear the link once the field passes
		attrs["aria-describedby"] = id
		attrs["data-tc-described"] = id
	}
	return attrs
}

// FieldValue returns the value submitted for name after Failed, else fallback.
func FieldValue(ctx context.Context, name, fallback string) string {
	if f, ok := ctx.Value(failedKey{}).(*failedForm); ok {
		if v, sent := f.values[name]; sent && len(v) > 0 {
			return v[0]
		}
	}
	return fallback
}

// FieldValues returns every value submitted for name after Failed, else
// fallback; a cleared group yields nil, not fallback.
func FieldValues(ctx context.Context, name string, fallback []string) []string {
	if f, ok := ctx.Value(failedKey{}).(*failedForm); ok {
		return f.values[name]
	}
	return fallback
}
