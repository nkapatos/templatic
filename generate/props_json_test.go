package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func convertTS(t *testing.T, src, name string) (string, *propsTypes) {
	t.Helper()
	out, err := convertProps(t, writePkg(t, src), name)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, n := range out.Names {
		sb.WriteString("export interface " + n + " {\n" + out.Interfaces[n] + "}\n")
	}
	return sb.String(), out
}

func typecheck(t *testing.T, ts string) {
	t.Helper()
	tsc, err := filepath.Abs("../internal/clientruntime/node_modules/.bin/tsc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tsc); err != nil {
		t.Skip("tsc not installed")
	}
	file := filepath.Join(t.TempDir(), "types.ts")
	if err := os.WriteFile(file, []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tsc, "--noEmit", "--strict", "--target", "es2020", file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tsc rejects the generated types: %v\n%s\n%s", err, out, ts)
	}
}

func TestPropsJSONRules(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		root   string
		want   []string
		absent []string
		fields string
	}{
		{
			name: "non-identifier names are quoted",
			src:  "type P struct {\n\tFirst string `json:\"first-name\"`\n\tB int `json:\"a b\"`\n\tC int `json:\"$ok\"`\n}",
			root: "P", want: []string{`"first-name": string;`, `"a b": number;`, `$ok: number;`},
			fields: "first-name,a b,$ok",
		},
		{
			name: "only the json key is read",
			src:  "type P struct {\n\tA string `xjson:\"nope\"`\n\tB string `yaml:\"y\" json:\"b\"`\n}",
			root: "P", want: []string{"A: string;", "b: string;"}, absent: []string{"nope"},
			fields: "A,b",
		},
		{
			name: "dash comma is a field named dash",
			src:  "type P struct {\n\tA int `json:\"-,\"`\n\tB int `json:\"-\"`\n}",
			root: "P", want: []string{`"-": number;`}, absent: []string{"B:"},
			fields: "-",
		},
		{
			name: "omitting a group drops every name",
			src:  "type P struct {\n\tX, Y int `json:\"-\"`\n\tZ int\n}",
			root: "P", want: []string{"Z: number;"}, absent: []string{"X:", "Y:"},
			fields: "Z",
		},
		{
			name: "invalid tag name falls back to the field name",
			src:  "type P struct {\n\tA int `json:\"a\\\\b\"`\n\tB int `json:\",omitempty\"`\n}",
			root: "P", want: []string{"A: number;", "B: number;"},
			fields: "A,B",
		},
		{
			name: "untagged embedded struct is promoted",
			src: "type Base struct {\n\tID int `json:\"id\"`\n\tCreated string\n}\n" +
				"type P struct {\n\tBase\n\tName string\n}",
			root: "P", want: []string{"id: number;", "Created: string;", "Name: string;"},
			fields: "id,Created,Name",
		},
		{
			name: "tagged embedded struct is one field",
			src: "type Base struct{ ID int }\n" +
				"type P struct {\n\tBase `json:\"base\"`\n\tName string\n}",
			root: "P", want: []string{"base: Base;", "Name: string;"},
			fields: "base,Name",
		},
		{
			name: "pointer and nested embedding, shallower field wins",
			src: "type Inner struct {\n\tName string\n\tDeep int\n}\n" +
				"type Mid struct {\n\t*Inner\n\tName int\n}\n" +
				"type P struct {\n\tMid\n}",
			root: "P", want: []string{"Name: number;", "Deep: number;"}, absent: []string{"Name: string;"},
			fields: "Deep,Name",
		},
		{
			name: "same depth conflict drops both, a tag breaks the tie",
			src: "type A struct {\n\tX int\n\tY int\n}\n" +
				"type B struct {\n\tX string\n\tY string `json:\"Y\"`\n}\n" +
				"type P struct {\n\tA\n\tB\n}",
			root: "P", want: []string{"Y: string;"}, absent: []string{"X:", "Y: number;"},
			fields: "Y",
		},
		{
			name: "embedded cycle terminates",
			src:  "type A struct {\n\t*A\n\tV int\n}",
			root: "A", want: []string{"V: number;"},
			fields: "V",
		},
		{
			name: "omitempty and omitzero make a field optional",
			src:  "type P struct {\n\tA string `json:\"a,omitempty\"`\n\tB int `json:\",omitzero\"`\n\tC []int `json:\"c,omitempty\"`\n\tD int\n}",
			root: "P", want: []string{"a?: string;", "B?: number;", "c?: number[];", "D: number;"},
			fields: "a,B,c,D",
		},
		{
			name: "omitempty never omits numbers and bools",
			src:  "type P struct {\n\tN int `json:\",omitempty\"`\n\tB bool `json:\",omitempty\"`\n\tP *int `json:\",omitempty\"`\n\tM map[string]int `json:\",omitempty\"`\n}",
			root: "P", want: []string{"N: number;", "B: boolean;", "P?: number;", "M?: Record<string, number>;"},
			fields: "N,B,P,M",
		},
		{
			name: "pointers are nullable unless optional",
			src:  "type In struct{ X int }\ntype P struct {\n\tA *In\n\tB *In `json:\"b,omitempty\"`\n\tC *int `json:\"c,omitzero\"`\n\tD []*In\n\tE map[string]*int\n\tF **int\n}",
			root: "P", want: []string{"A: In | null;", "b?: In;", "c?: number;", "D: (In | null)[];", "E: Record<string, number | null>;", "F: number | null;"},
			absent: []string{"b?: In | null", "c?: number | null"},
			fields: "A,b,c,D,E,F",
		},
		{
			name: "nil slices and maps stay plain",
			src:  "type P struct {\n\tS []string\n\tM map[string]int\n}",
			root: "P", want: []string{"S: string[];", "M: Record<string, number>;"}, absent: []string{"null"},
			fields: "S,M",
		},
		{
			name: "byte arrays and slices are base64 strings",
			src:  "type P struct {\n\tA [4]byte\n\tB []byte\n\tC [2]int\n\tD *[3]byte\n}",
			root: "P", want: []string{"A: string;", "B: string;", "C: number[];", "D: string | null;"},
			fields: "A,B,C,D",
		},
		{
			name: "string option quotes numbers only",
			src:  "type P struct {\n\tA int64 `json:\",string\"`\n\tB float64 `json:\"b,string\"`\n\tC *int `json:\"c,string\"`\n\tD []int `json:\",string\"`\n}",
			root: "P", want: []string{"A: string;", "b: string;", "c: string | null;", "D: number[];"},
			fields: "A,b,c,D",
		},
		{
			name: "embed option promotes a named field",
			src:  "type Base struct{ ID int }\ntype P struct {\n\tB Base `json:\",embed\"`\n\tName string\n}",
			root: "P", want: []string{"ID: number;", "Name: string;"}, absent: []string{"B:"},
			fields: "ID,Name",
		},
		{
			name: "output follows depth-first order, not declaration order",
			src:  "type Base struct{ Z int }\ntype P struct {\n\tA int\n\tBase\n\tC int\n}",
			root: "P", want: []string{"A: number;", "Z: number;", "C: number;"},
			fields: "A,Z,C",
		},
		{
			name: "a type embedded twice is read at both places, its own embeds once",
			src: "type D struct{ Deep int }\ntype A struct {\n\tD\n\tX int\n}\n" +
				"type B struct{ A }\ntype C struct{ A }\ntype P struct {\n\tB\n\tC\n}",
			root: "P", want: []string{"Deep: number;"}, absent: []string{"X:"},
			fields: "Deep",
		},
		{
			name: "map keys",
			src: "type P struct {\n\tS map[string]int\n\tI map[int64]bool\n\tU map[uint8]string\n" +
				"\tF map[float64]int\n\tN map[Name]int\n\tT map[time.Time]int\n}\ntype Name string",
			root: "P", want: []string{"S: Record<string, number>;", "I: Record<number, boolean>;",
				"U: Record<number, string>;", "F: Record<string, number>;", "N: Record<string, number>;", "T: Record<string, number>;"},
			absent: []string{"unknown"},
			fields: "S,I,U,F,N,T",
		},
		{
			name: "anonymous struct fields follow the same rules",
			src:  "type P struct {\n\tA struct {\n\t\tX, Y int `json:\"-\"`\n\t\tK int `json:\"k-1\"`\n\t}\n}",
			root: "P", want: []string{`"k-1": number;`}, absent: []string{"X:", "Y:"},
			fields: "A",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			imports := ""
			if strings.Contains(tc.src, "time.Time") {
				imports = "import \"time\"\n"
			}
			ts, out := convertTS(t, "package app\n"+imports+tc.src+"\n", tc.root)
			for _, w := range tc.want {
				if !strings.Contains(ts, w) {
					t.Errorf("missing %q in:\n%s", w, ts)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(ts, a) {
					t.Errorf("unexpected %q in:\n%s", a, ts)
				}
			}
			if got := strings.Join(out.Fields[tc.root], ","); got != tc.fields {
				t.Errorf("Fields = %q, want %q", got, tc.fields)
			}
			typecheck(t, ts)
		})
	}
}

func TestNonIdentifierJSONNameIsJSStringLiteral(t *testing.T) {
	// v2 accepts any name without quotes or backslashes, "😀" included; TS gets it as a plain string literal.
	ts, _ := convertTS(t, "package p\ntype P struct {\n\tA string `json:\"a-b\"`\n\tB string `json:\"é\"`\n\tC string `json:\"c<d\"`\n\tD string `json:\"😀\"`\n}\n", "P")
	for _, want := range []string{"\"a-b\": string;", "\"é\": string;", "\"c<d\": string;", "\"😀\": string;"} {
		if !strings.Contains(ts, want) {
			t.Errorf("missing %q in:\n%s", want, ts)
		}
	}
	typecheck(t, ts)
}
