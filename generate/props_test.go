package generate

import (
	"strings"
	"testing"

	"github.com/nkapatos/templatic/internal/testapp"
)

func writePkg(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	if strings.Contains(src, templaticImport) {
		testapp.WriteModule(t, dir, "example.test/app")
	} else {
		writeFiles(t, dir, map[string]string{"go.mod": "module example.test/app\n\ngo 1.27.4\n"})
	}
	writeFiles(t, dir, map[string]string{"fixture.go": src})
	return dir
}

func convertProps(t *testing.T, dir, name string) (*propsTypes, error) {
	t.Helper()
	return convertPropsTypes(mustLoad(t, dir), ".", name)
}

func TestPropsHandlesSelfReferentialStructs(t *testing.T) {
	dir := writePkg(t, `package app

type Node struct {
	Name string
	Kids []Node
	Up   *Node
}

type Pair struct {
	A B
	B B
}

type B struct {
	Mine int
	Next *B
}
`)
	out, err := convertProps(t, dir, "Node")
	if err != nil {
		t.Fatalf("self-referential props must convert, not recurse forever: %v", err)
	}
	body := out.Interfaces["Node"]
	if !strings.Contains(body, "Kids: Node[];") || !strings.Contains(body, "Up: Node | null;") {
		t.Fatalf("Node interface wrong: %q", body)
	}
	if len(out.Names) != 1 || out.Names[0] != "Node" {
		t.Fatalf("referenced-type order wrong: %v", out.Names)
	}

	out, err = convertProps(t, dir, "Pair")
	if err != nil {
		t.Fatalf("mutually recursive props must convert: %v", err)
	}
	if want := []string{"Pair", "B"}; len(out.Names) != 2 || out.Names[0] != want[0] || out.Names[1] != want[1] {
		t.Fatalf("referenced-type order wrong: %v want %v", out.Names, want)
	}
	if b := out.Interfaces["B"]; !strings.Contains(b, "Next: B | null;") {
		t.Fatalf("B interface wrong: %q", b)
	}
	if !strings.Contains(out.Interfaces["Pair"], "B: B;") {
		t.Fatalf("Pair interface wrong: %q", out.Interfaces["Pair"])
	}
}

func TestPropsMultiNameFieldsKeepEveryName(t *testing.T) {
	// A tag applies to every name of a field group, so they conflict and neither is marshaled.
	dir := writePkg(t, `package app

type Range struct {
	X, Y    int
	Title   string
	Lat, Lng float64 `+"`json:\"lat,lon\"`"+`
}
`)
	out, err := convertProps(t, dir, "Range")
	if err != nil {
		t.Fatal(err)
	}
	body := out.Interfaces["Range"]
	for _, want := range []string{"X: number;", "Y: number;", "Title: string;"} {
		if !strings.Contains(body, want) {
			t.Fatalf("Range interface lost a field (%q): %q", want, body)
		}
	}
	if strings.Contains(body, "lat") || strings.Contains(body, "Lng") {
		t.Fatalf("conflicting tagged names must both be dropped: %q", body)
	}
	if fields := out.Fields["Range"]; strings.Join(fields, ",") != "X,Y,Title" {
		t.Fatalf("field names wrong: %v", fields)
	}
}

func TestPropsMapsLiveStringsToStringArray(t *testing.T) {
	dir := writePkg(t, `package app

import "github.com/nkapatos/templatic/live"

type P struct {
	Tags live.Strings `+"`json:\"tags\"`"+`
}
`)
	out, err := convertProps(t, dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if body := out.Interfaces["P"]; !strings.Contains(body, "tags: string[];") {
		t.Fatalf("live.Strings must type as string[]: %q (warnings %v)", body, out.Warnings)
	}
}

func TestPropsMapsLiveSnapshotToItsItemsArray(t *testing.T) {
	dir := writePkg(t, `package app

import "github.com/nkapatos/templatic/live"

type Row struct {
	ID   int    `+"`json:\"id\"`"+`
	Text string `+"`json:\"text\"`"+`
}

type P struct {
	Rows live.Snapshot[Row] `+"`json:\"rows\"`"+`
}
`)
	out, err := convertProps(t, dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if body := out.Interfaces["P"]; !strings.Contains(body, "rows: Row[];") {
		t.Fatalf("live.Snapshot[Row] must type as Row[]: %q (warnings %v)", body, out.Warnings)
	}
	if body := out.Interfaces["Row"]; !strings.Contains(body, "id: number;") {
		t.Fatalf("the item type must be emitted: %q", body)
	}
}

func TestPropsFollowJSONMarshalers(t *testing.T) {
	dir := writePkg(t, `package app

import (
	"encoding/json/jsontext"
	"net/netip"
)

type Money struct{ cents int }

func (Money) MarshalJSON() ([]byte, error) { return nil, nil }

type P struct {
	Blob  []byte
	Fixed [2]byte
	Raw   jsontext.Value
	Addr  netip.Addr
	Price Money
}
`)
	out, err := convertProps(t, dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	body := out.Interfaces["P"]
	for _, want := range []string{"Blob: string;", "Fixed: string;", "Raw: unknown;", "Addr: string;", "Price: unknown;"} {
		if !strings.Contains(body, want) {
			t.Errorf("want %q in %q", want, body)
		}
	}
	if _, ok := out.Interfaces["Money"]; ok {
		t.Error("a type with MarshalJSON must not be expanded into an interface")
	}
}

func TestPropsRejectWhatJSONv2CannotMarshal(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"package app\n\nimport \"time\"\n\ntype P struct{ Wait []time.Duration }\n", "field Wait: time.Duration has no json/v2 encoding"},
		{"package app\n\ntype Name string\n\ntype P struct {\n\tName\n\tX int\n}\n", "embedded field Name of non-struct type needs a JSON name"},
		{"package app\n\ntype name string\n\ntype P struct {\n\t*name\n}\n", "embedded field name of non-struct type needs a JSON name"},
	} {
		_, err := convertProps(t, writePkg(t, tc.src), "P")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("want error %q, got %v", tc.want, err)
		}
	}
	if _, err := convertProps(t, writePkg(t, "package app\n\ntype Name string\n\ntype P struct {\n\tName `json:\"n\"`\n}\n"), "P"); err != nil {
		t.Errorf("a named embedded non-struct marshals: %v", err)
	}
}
