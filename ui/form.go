package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"

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

// FieldError renders the error slot of field name; place it inside the form.
func FieldError(name string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		msg := ""
		if f, ok := ctx.Value(failedKey{}).(*failedForm); ok {
			msg = f.fields[name]
		}
		_, err := fmt.Fprintf(w, `<span data-tc-field-error="%s" aria-live="polite">%s</span>`,
			templ.EscapeString(name), templ.EscapeString(msg))
		return err
	})
}

// FormError renders the error slot for errors that name no field.
func FormError() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		msg := ""
		if f, ok := ctx.Value(failedKey{}).(*failedForm); ok {
			msg = f.message
		}
		_, err := fmt.Fprintf(w, `<span data-tc-form-error aria-live="polite">%s</span>`, templ.EscapeString(msg))
		return err
	})
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
