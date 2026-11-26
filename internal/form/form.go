// Package form decodes submitted forms into a command's input struct.
package form

import (
	"encoding"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// FieldError is one refused field; Name is the field's json name.
type FieldError struct {
	Name    string
	Message string
}

type field struct {
	name  string
	index []int
	kind  fieldKind
	typ   reflect.Type
}

type fieldKind int

const (
	scalar fieldKind = iota
	optional
	list
)

type plan struct {
	fields []field
	err    error
}

var plans sync.Map

// Check reports whether forms can fill t; the error names the first field none can.
func Check(t reflect.Type) error { return planFor(t).err }

// Names lists the form names of t's fields in field order.
func Names(t reflect.Type) []string {
	p := planFor(t)
	out := make([]string, len(p.fields))
	for i, f := range p.fields {
		out[i] = f.name
	}
	return out
}

// Decode fills dst, a pointer to a struct, from vals and lists each refused
// field; a non-nil error means dst's type cannot be filled by a form.
func Decode(vals url.Values, dst any) ([]FieldError, error) {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil, fmt.Errorf("form: decode needs a non-nil pointer, got %T", dst)
	}
	p := planFor(v.Type())
	if p.err != nil {
		return nil, p.err
	}
	v = v.Elem()
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	var errs []FieldError
	for _, f := range p.fields {
		got, present := vals[f.name]
		if !present {
			continue
		}
		if msg := f.set(v.FieldByIndex(f.index), got); msg != "" {
			errs = append(errs, FieldError{Name: f.name, Message: msg})
		}
	}
	return errs, nil
}

func planFor(t reflect.Type) *plan {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if p, ok := plans.Load(t); ok {
		return p.(*plan)
	}
	p := &plan{}
	if t.Kind() != reflect.Struct {
		p.err = fmt.Errorf("form: input %s is not a struct", t)
	} else {
		seen := map[string]bool{}
		p.err = collect(t, nil, seen, &p.fields)
	}
	got, _ := plans.LoadOrStore(t, p)
	return got.(*plan)
}

// collect rejects a name that appears twice rather than applying json's dominance
// rules.
func collect(t reflect.Type, prefix []int, seen map[string]bool, out *[]field) error {
	for i := range t.NumField() {
		sf := t.Field(i)
		tag := sf.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if tag == "-" {
			continue
		}
		index := append(append([]int(nil), prefix...), i)
		if sf.Anonymous && name == "" && sf.Type.Kind() == reflect.Struct {
			if err := collect(sf.Type, index, seen, out); err != nil {
				return err
			}
			continue
		}
		if !sf.IsExported() {
			continue
		}
		if name == "" {
			name = sf.Name
		}
		kind, ok := kindOf(sf.Type)
		if !ok {
			return fmt.Errorf("form: field %s (%s) of %s is not a form field: forms carry strings, numbers, bools, encoding.TextUnmarshaler values, pointers to them and slices of them", sf.Name, sf.Type, t)
		}
		if seen[name] {
			return fmt.Errorf("form: %s has two fields named %q", t, name)
		}
		seen[name] = true
		*out = append(*out, field{name: name, index: index, kind: kind, typ: sf.Type})
	}
	return nil
}

var textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()

func kindOf(t reflect.Type) (fieldKind, bool) {
	switch {
	case isScalar(t):
		return scalar, true
	case t.Kind() == reflect.Pointer && isScalar(t.Elem()):
		return optional, true
	case t.Kind() == reflect.Slice && isScalar(t.Elem()) && t.Elem().Kind() != reflect.Uint8:
		return list, true
	}
	return 0, false
}

func isScalar(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(textUnmarshaler) {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func (f field) set(v reflect.Value, vals []string) string {
	switch f.kind {
	case optional:
		if len(vals) > 1 {
			return "expected one value"
		}
		if len(vals) == 0 || vals[0] == "" {
			v.SetZero()
			return ""
		}
		elem := reflect.New(f.typ.Elem())
		if msg := setScalar(elem.Elem(), vals[0]); msg != "" {
			return msg
		}
		v.Set(elem)
		return ""
	case list:
		s := reflect.MakeSlice(f.typ, 0, len(vals))
		for _, raw := range vals {
			if raw == "" && f.typ.Elem().Kind() != reflect.String {
				continue
			}
			elem := reflect.New(f.typ.Elem()).Elem()
			if msg := setScalar(elem, raw); msg != "" {
				return msg
			}
			s = reflect.Append(s, elem)
		}
		v.Set(s)
		return ""
	case scalar:
		if len(vals) == 0 {
			return ""
		}
		raw := vals[0]
		if len(vals) > 1 {
			// A checkbox after a hidden input of the same name sends both; the last is
			// the box.
			if f.typ.Kind() != reflect.Bool {
				return "expected one value"
			}
			raw = vals[len(vals)-1]
		}
		if raw == "" && f.typ.Kind() != reflect.String {
			v.SetZero()
			return ""
		}
		return setScalar(v, raw)
	}
	return ""
}

func setScalar(v reflect.Value, raw string) string {
	if u, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
		if err := u.UnmarshalText([]byte(raw)); err != nil {
			return "is not valid"
		}
		return ""
	}
	s := strings.TrimSpace(raw)
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		switch strings.ToLower(s) {
		case "on", "true", "1":
			v.SetBool(true)
		case "off", "false", "0", "":
			v.SetBool(false)
		default:
			return "must be true or false"
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, v.Type().Bits())
		if err != nil {
			return numberMessage(err, "must be a whole number")
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, v.Type().Bits())
		if err != nil {
			return numberMessage(err, "must be a whole number, 0 or more")
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(s, v.Type().Bits())
		if err != nil {
			return numberMessage(err, "must be a number")
		}
		// ParseFloat reads "NaN" and "Inf", which no number input sends.
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return "must be a number"
		}
		v.SetFloat(n)
	}
	return ""
}

func numberMessage(err error, syntax string) string {
	if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
		return "is out of range"
	}
	return syntax
}

// SubmitField is the hidden field that carries a form's submission id.
const SubmitField = "_tc_submit"
