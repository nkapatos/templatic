package live

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type day struct{ time.Time }

func (d *day) UnmarshalText(b []byte) error {
	t, err := time.Parse("2006-01-02", string(b))
	d.Time = t
	return err
}

type orderIn struct {
	Title  string   `json:"title"`
	Qty    int      `json:"qty"`
	Max    *int     `json:"max"`
	Price  float64  `json:"price"`
	Gift   bool     `json:"gift"`
	Tags   []string `json:"tag"`
	Sizes  []int    `json:"size"`
	Due    day      `json:"due"`
	Ignore string   `json:"-"`
}

func (in orderIn) Validate() error {
	var errs []error
	if strings.TrimSpace(in.Title) == "" {
		errs = append(errs, Invalid("title", "required"))
	}
	if in.Qty < 0 {
		errs = append(errs, Invalid("qty", "cannot be negative"))
	}
	return errors.Join(errs...)
}

// submitBoth posts form to cmd.Form without JavaScript and as the runtime does; it returns the no-JS input,
// the runtime's response and the no-JS error, and fails when the two transports disagree.
func submitBoth(t *testing.T, form url.Values) (orderIn, *http.Response, error) {
	t.Helper()
	cmd := NewCommand[orderIn]("order.place")
	noJS, noJSErr := cmd.Decode(formReq(form))

	w := NewWorld()
	var viaJS orderIn
	h := cmd.Form(w, func(_ *http.Request, _ *Session, in orderIn) (string, error) { viaJS = in; return "/", nil }, nil)
	ts := serveWorld(t, w)
	st := openStream(t, ts, "form-sid")
	t.Cleanup(func() { st.Close() })
	st.discardUntilReady(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsFormReq("form-sid", form))
	resp := rec.Result()
	t.Cleanup(func() { resp.Body.Close() })
	if noJSErr == nil && resp.StatusCode == http.StatusNoContent && !reflect.DeepEqual(noJS, viaJS) {
		t.Fatalf("transports disagree:\nno-JS %+v\nJS    %+v", noJS, viaJS)
	}
	if (noJSErr == nil) != (resp.StatusCode == http.StatusNoContent) {
		t.Fatalf("transports disagree: no-JS err %v, JS status %d", noJSErr, resp.StatusCode)
	}
	return noJS, resp, noJSErr
}

func TestFormFieldsDecodeByType(t *testing.T) {
	in, _, err := submitBoth(t, url.Values{
		"title": {"Mugs"}, "qty": {" 3 "}, "max": {"10"}, "price": {"2.5"},
		"gift": {"on"}, "tag": {"a", "b"}, "size": {"1", "", "3"}, "due": {"2026-12-01"},
		"Ignore": {"x"}, "unknown": {"y"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if in.Title != "Mugs" || in.Qty != 3 || in.Max == nil || *in.Max != 10 || in.Price != 2.5 ||
		!in.Gift || len(in.Tags) != 2 || fmt.Sprint(in.Sizes) != "[1 3]" ||
		in.Due.Format("2006-01-02") != "2026-12-01" || in.Ignore != "" {
		t.Fatalf("%+v", in)
	}
}

func TestFormEmptyAndAbsentValues(t *testing.T) {
	in, _, err := submitBoth(t, url.Values{"title": {"x"}, "qty": {""}, "max": {""}, "due": {""}, "tag": {"only"}})
	if err != nil {
		t.Fatal(err)
	}
	if in.Qty != 0 || in.Max != nil || in.Gift || len(in.Tags) != 1 || !in.Due.IsZero() {
		t.Fatalf("%+v", in)
	}
	if in, _, err := submitBoth(t, url.Values{"title": {"x"}, "gift": {"off", "on"}}); err != nil || !in.Gift {
		t.Fatalf("hidden+checkbox: %+v %v", in, err)
	}
	if in, _, err := submitBoth(t, url.Values{"title": {"x"}, "gift": {"off"}}); err != nil || in.Gift {
		t.Fatalf("hidden only: %+v %v", in, err)
	}
}

func TestFormReportsEveryField(t *testing.T) {
	_, resp, err := submitBoth(t, url.Values{
		"title": {""}, "qty": {"three"}, "price": {"NaN"}, "gift": {"maybe"}, "max": {"1", "2"}, "due": {"soon"},
	})
	want := map[string]string{
		"title": "required",
		"qty":   "must be a whole number",
		"price": "must be a number",
		"gift":  "must be true or false",
		"max":   "expected one value",
		"due":   "is not valid",
	}
	got := map[string]string{}
	for _, ie := range InputErrors(err) {
		got[ie.Field] = ie.Message
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("no-JS errors\n got %v\nwant %v", got, want)
	}
	if env := errorBody(t, resp); fmt.Sprint(env.Fields) != fmt.Sprint(want) || env.Message != "" {
		t.Fatalf("JS envelope %+v", env)
	}
	_, _, err = submitBoth(t, url.Values{"title": {"x"}, "qty": {"-1"}})
	if ies := InputErrors(err); len(ies) != 1 || ies[0].Field != "qty" || ies[0].Message != "cannot be negative" {
		t.Fatalf("%v", err)
	}
	_, _, err = submitBoth(t, url.Values{"title": {"x"}, "qty": {"99999999999999999999"}})
	if ies := InputErrors(err); len(ies) != 1 || ies[0].Message != "is out of range" {
		t.Fatalf("%v", err)
	}
}

func TestTypedCallStaysJSON(t *testing.T) {
	w := NewWorld()
	var got orderIn
	NewCommand[orderIn]("order.place").Handle(w, func(_ *Session, in orderIn) (any, error) { got = in; return nil, nil })
	ts := serveWorld(t, w)
	st := openStream(t, ts, "json-sid")
	defer st.Close()
	st.discardUntilReady(t)
	if resp := postCommand(t, ts, "json-sid", "order.place", jsontext.Value(`{"title":"x","qty":2}`)); resp.StatusCode != http.StatusNoContent || got.Qty != 2 {
		t.Fatalf("typed call: %d %+v", resp.StatusCode, got)
	}
	if e := errorBody(t, postCommand(t, ts, "json-sid", "order.place", jsontext.Value(`{"title":"x","qty":"2"}`))); e.Fields["qty"] == "" {
		t.Fatalf("%+v", e)
	}
}

func TestFormRefusesNonFormInput(t *testing.T) {
	type nested struct {
		Addr struct{ City string } `json:"addr"`
	}
	_, err := NewCommand[nested]("x.y").Decode(formReq(url.Values{"addr": {"z"}}))
	if err == nil || len(InputErrors(err)) != 0 || !strings.Contains(err.Error(), "not a form field") {
		t.Fatalf("%v", err)
	}
	type inner struct{ A string }
	type dup struct {
		A string
		inner
	}
	if _, err := NewCommand[dup]("x.z").Decode(formReq(url.Values{})); err == nil || !strings.Contains(err.Error(), `two fields named "A"`) {
		t.Fatalf("%v", err)
	}
}

func TestFormPromotesEmbedded(t *testing.T) {
	type base struct {
		ID string `json:"id"`
	}
	type in struct {
		base
		N int `json:"n"`
	}
	got, err := NewCommand[in]("x.e").Decode(formReq(url.Values{"id": {"7"}, "n": {"2"}}))
	if err != nil || got.ID != "7" || got.N != 2 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestFormListsEveryError(t *testing.T) {
	h := NewCommand[orderIn]("order.place").Form(NewWorld(), func(*http.Request, *Session, orderIn) (string, error) { return "/", nil }, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, formReq(url.Values{"qty": {"x"}}))
	body, _ := io.ReadAll(rec.Result().Body)
	if rec.Code != http.StatusBadRequest || !strings.Contains(string(body), "title: required") || !strings.Contains(string(body), "qty: must be a whole number") {
		t.Fatalf("%d %q", rec.Code, body)
	}
}

func TestFormPointerInput(t *testing.T) {
	type in struct {
		N int `json:"n"`
	}
	got, err := NewCommand[*in]("x.p").Decode(formReq(url.Values{"n": {"2"}}))
	if err != nil || got == nil || got.N != 2 {
		t.Fatalf("no-JS: %+v %v", got, err)
	}
	got, err = decodeInput[*in](formArgs(url.Values{}))
	if err != nil || got == nil {
		t.Fatalf("empty form: %+v %v", got, err)
	}
}

func TestFormBodyTooLarge(t *testing.T) {
	h := http.MaxBytesHandler(NewCommand[orderIn]("order.big").Form(NewWorld(), func(*http.Request, *Session, orderIn) (string, error) { return "/", nil }, nil), 64)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, formReq(url.Values{"title": {strings.Repeat("x", 200)}}))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
}

func TestCommandBodyTooLarge(t *testing.T) {
	w := NewWorld()
	rec := httptest.NewRecorder()
	body := `{"sid":"s","action":"a","args":"` + strings.Repeat("x", maxCommandBytes) + `"}`
	w.Commands().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
}

func TestDecodeFormMalformedBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", strings.NewReader("title=%zz"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := NewCommand[orderIn]("x.m").Decode(r); len(InputErrors(err)) != 1 || InputErrors(err)[0].Message != "malformed form" {
		t.Fatalf("got %v", err)
	}
}

func TestJSONPointerInput(t *testing.T) {
	type in struct {
		N int `json:"n"`
	}
	for _, raw := range []string{"", "null"} {
		got, err := decodeInput[*in](jsonArgs(jsontext.Value(raw)))
		if err != nil || got == nil {
			t.Fatalf("args %q: %+v %v", raw, got, err)
		}
	}
}

func TestCommandBodyRejectsTrailingData(t *testing.T) {
	w := NewWorld()
	rec := httptest.NewRecorder()
	w.Commands().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"sid":"s","action":"a"}{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestJSONInputErrorFieldIsThePointerPath(t *testing.T) {
	type address struct {
		Zip int `json:"zip"`
	}
	type in struct {
		Count   int     `json:"count"`
		Address address `json:"address"`
	}
	for _, c := range []struct{ raw, field, msg string }{
		{`{"count":"x"}`, "count", "expected int"},
		{`{"address":{"zip":"x"}}`, "address.zip", "expected int"},
		{`{"count":`, "", "malformed input"},
	} {
		var v in
		ies := InputErrors(jsonArgs(jsontext.Value(c.raw)).Into(&v))
		if len(ies) != 1 || ies[0].Field != c.field || ies[0].Message != c.msg {
			t.Fatalf("%s: got %+v, want %q %q", c.raw, ies, c.field, c.msg)
		}
	}
}
