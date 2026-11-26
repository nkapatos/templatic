package live

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/nkapatos/templatic/internal/form"
)

// Args is a command's input as the client sent it: JSON or submitted form fields.
type Args struct {
	raw    jsontext.Value
	form   url.Values
	isForm bool
}

func jsonArgs(raw jsontext.Value) Args { return Args{raw: raw} }

// formArgs drops the submission id, which is the transport's, not the input's.
func formArgs(vals url.Values) Args {
	vals.Del(form.SubmitField)
	return Args{form: vals, isForm: true}
}

// Into decodes the args into v, a pointer.
func (a Args) Into(v any) error {
	if a.isForm {
		errs, err := form.Decode(a.form, v)
		if err != nil {
			return err
		}
		out := make([]error, len(errs))
		for i, fe := range errs {
			out[i] = &InputError{Field: fe.Name, Message: fe.Message}
		}
		return errors.Join(out...)
	}
	if len(a.raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(a.raw, v); err != nil {
		var se *json.SemanticError
		if errors.As(err, &se) && se.GoType != nil {
			return &InputError{Field: strings.Join(slices.Collect(se.JSONPointer.Tokens()), "."), Message: "expected " + se.GoType.String()}
		}
		return &InputError{Message: "malformed input"}
	}
	return nil
}

// Form returns the submitted fields when the args came from a form.
func (a Args) Form() (url.Values, bool) { return a.form, a.isForm }

// InputErrors returns every *InputError in err's tree, in order.
func InputErrors(err error) []*InputError {
	var out []*InputError
	var walk func(error)
	walk = func(e error) {
		switch u := e.(type) {
		case nil:
		case *InputError:
			out = append(out, u)
		case interface{ Unwrap() []error }:
			for _, x := range u.Unwrap() {
				walk(x)
			}
		case interface{ Unwrap() error }:
			walk(u.Unwrap())
		}
	}
	walk(err)
	return out
}

// Validate also runs when some fields were refused, so one submit reports every problem.
func decodeInput[In any](args Args) (In, error) {
	var in In
	derr := args.Into(&in)
	allocPointers(&in)
	decoded := InputErrors(derr)
	if derr != nil && len(decoded) == 0 {
		return in, derr
	}
	for _, ie := range decoded {
		if ie.Field == "" {
			return in, derr
		}
	}
	verr := validate(&in)
	switch {
	case derr == nil:
		return in, verr
	case verr == nil:
		return in, derr
	}
	validated := InputErrors(verr)
	if len(validated) == 0 {
		return in, derr
	}
	refused := map[string]bool{}
	for _, ie := range decoded {
		refused[ie.Field] = true
	}
	all := []error{derr}
	for _, ie := range validated {
		if ie.Field == "" || !refused[ie.Field] {
			all = append(all, ie)
		}
	}
	return in, errors.Join(all...)
}

func allocPointers[In any](in *In) {
	for v := reflect.ValueOf(in).Elem(); v.Kind() == reflect.Pointer; v = v.Elem() {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
	}
}

func validate[In any](in *In) error {
	if v, ok := any(in).(Validator); ok {
		return v.Validate()
	}
	if v, ok := any(*in).(Validator); ok {
		return v.Validate()
	}
	return nil
}

type inputEnvelope struct {
	Message string            `json:"message,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// WriteInputError answers err with the 400 envelope and reports whether err held
// input errors.
func WriteInputError(w http.ResponseWriter, err error) bool {
	ies := InputErrors(err)
	if len(ies) == 0 {
		return false
	}
	env := inputEnvelope{}
	var whole []string
	for _, ie := range ies {
		if ie.Field == "" {
			whole = append(whole, ie.Message)
			continue
		}
		if env.Fields == nil {
			env.Fields = map[string]string{}
		}
		if _, dup := env.Fields[ie.Field]; !dup {
			env.Fields[ie.Field] = ie.Message
		}
	}
	env.Message = strings.Join(whole, "; ")
	payload, merr := json.Marshal(struct {
		Error inputEnvelope `json:"error"`
	}{env})
	if merr != nil {
		panic(fmt.Sprintf("live: input envelope: %v", merr))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(append(payload, '\n'))
	return true
}
