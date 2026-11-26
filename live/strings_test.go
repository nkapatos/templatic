package live

import (
	"encoding/json/jsontext"
	"testing"
)

func TestStringsDecodesEveryFormShape(t *testing.T) {
	type in struct {
		Tag Strings `json:"tag"`
	}
	cases := []struct {
		args string
		want []string
	}{
		{`{"tag":"a"}`, []string{"a"}},
		{`{"tag":["a","b"]}`, []string{"a", "b"}},
		{`{"tag":[]}`, []string{}},
		{`{"tag":null}`, nil},
		{`{}`, nil},
		{`{"tag":""}`, []string{""}},
	}
	for _, c := range cases {
		var v in
		if err := jsonArgs(jsontext.Value(c.args)).Into(&v); err != nil {
			t.Fatalf("%s: %v", c.args, err)
		}
		if len(v.Tag) != len(c.want) {
			t.Fatalf("%s: got %q, want %q", c.args, v.Tag, c.want)
		}
		for i := range c.want {
			if v.Tag[i] != c.want[i] {
				t.Fatalf("%s: got %q, want %q", c.args, v.Tag, c.want)
			}
		}
	}
}

func TestStringsRejectsNonStringShapes(t *testing.T) {
	type in struct {
		Tag Strings `json:"tag"`
	}
	for _, bad := range []string{`{"tag":1}`, `{"tag":[1]}`, `{"tag":{"a":"b"}}`} {
		var v in
		if err := jsonArgs(jsontext.Value(bad)).Into(&v); err == nil {
			t.Fatalf("%s: expected a decode error, got %q", bad, v.Tag)
		}
	}
}
