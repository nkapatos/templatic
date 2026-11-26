package generate

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nkapatos/templatic/internal/testapp"
)

const cardTempl = `package views

//templatic:island tag=x-card props=%s%s
templ Card(props %s) {
	<div></div>
}
`

func runModuleApp(t *testing.T, files map[string]string) (Options, error) {
	t.Helper()
	dir := t.TempDir()
	testapp.WriteModule(t, dir, "example.test/app")
	writeFiles(t, dir, files)
	writeFiles(t, dir, map[string]string{"client/islands/.keep": ""})
	opts := Options{Root: dir, SkipBundle: true}
	_, err := Run(opts)
	return opts, err
}

func TestPropsTypeFromAnotherPackage(t *testing.T) {
	opts, err := runModuleApp(t, map[string]string{
		"views/card.templ": fmt.Sprintf(cardTempl, "Props", "", "Props"),
		"views/props.go":   "package views\n\nimport \"example.test/app/model\"\n\ntype Props = model.Props\n",
		"model/model.go": `package model

import "example.test/app/geo"

type Props struct {
	Name  string    ` + "`json:\"name\"`" + `
	Where geo.Point ` + "`json:\"where\"`" + `
	geo.Label
}
`,
		"geo/geo.go": `package geo

type Point struct{ Lat, Lng float64 }

type Label struct{ Text string ` + "`json:\"text\"`" + ` }
`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(opts.Root, ".templatic", "islands", "x-card.types.ts"))
	for _, want := range []string{
		"export interface Props {\n  name: string;\n  where: Point;\n  text: string;\n}",
		"export interface Point {\n  Lat: number;\n  Lng: number;\n}",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "unknown") {
		t.Fatalf("cross-package types must resolve:\n%s", got)
	}
}

func bindApp(t *testing.T, topic string, files map[string]string) error {
	t.Helper()
	files["views/card.templ"] = fmt.Sprintf(cardTempl, "Props", " variant=lit bind=Name:"+topic, "Props")
	files["views/props.go"] = "package views\n\ntype Props struct{ Name string }\n"
	_, err := runModuleApp(t, files)
	return err
}

func TestBindCheckResolvesLiveCallsByFunction(t *testing.T) {
	files := func() map[string]string {
		return map[string]string{"cells/cells.go": `package cells

import (
	lv "github.com/nkapatos/templatic/live"
	. "github.com/nkapatos/templatic/live"
)

func f(w *lv.World) {
	lv.NewCell(w, "x.aliased", 0)
	NewCell(w, "x.dot", 0)
}
`, "other/live/live.go": "package live\n\nfunc NewCell(w any, topic string, v int) {}\n",
			"other/use.go": "package other\n\nimport \"example.test/app/other/live\"\n\nfunc g() { live.NewCell(nil, \"x.impostor\", 0) }\n"}
	}
	for _, topic := range []string{"x.aliased", "x.dot"} {
		if err := bindApp(t, topic, files()); err != nil {
			t.Fatalf("%s: %v", topic, err)
		}
	}
	err := bindApp(t, "x.impostor", files())
	if err == nil || !strings.Contains(err.Error(), "names no declared cell") {
		t.Fatalf("a function named NewCell in another package is not live.NewCell: %v", err)
	}
}

func TestBindCheckReadsConstTopicFromAnotherPackage(t *testing.T) {
	files := map[string]string{
		"topics/topics.go": "package topics\n\nconst prefix = \"x.\"\n\nconst Range = prefix + \"range\"\n",
		"cells/cells.go": `package cells

import (
	"github.com/nkapatos/templatic/live"

	"example.test/app/topics"
)

func f(w *live.World) { live.NewCell(w, topics.Range, 0) }
`,
	}
	if err := bindApp(t, "x.range", files); err != nil {
		t.Fatal(err)
	}
}

func TestCommandInputTypeFromAnotherPackage(t *testing.T) {
	dir := t.TempDir()
	testapp.WriteModule(t, dir, "example.test/app")
	gomod := read(t, filepath.Join(dir, "go.mod"))
	writeFiles(t, dir, map[string]string{
		"go.mod":         gomod + "\nrequire example.test/dep v0.0.0\n\nreplace example.test/dep => ./dep\n",
		"dep/go.mod":     "module example.test/dep\n\ngo 1.27.4\n",
		"dep/dep.go":     "package dep\n\ntype Ext struct {\n\tID int `json:\"id\"`\n}\n",
		"model/model.go": "package model\n\ntype Add struct {\n\tText string `json:\"text\"`\n}\n",
		"todo/todo.go": `package todo

import (
	"github.com/nkapatos/templatic/live"

	"example.test/app/model"
	"example.test/dep"
)

type Add = model.Add

var (
	A = live.NewCommand[Add]("todo.add")
	E = live.NewCommand[dep.Ext]("todo.ext")
)
`,
	})
	if err := writeCommandsTS(Options{Root: dir}, mustLoad(t, dir)); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(dir, ".templatic", "commands.ts"))
	for _, want := range []string{
		"export declare namespace model {\n\tinterface Add {\n\t\ttext: string;\n\t}\n}",
		"export declare namespace dep {\n\tinterface Ext {\n\t\tid: number;\n\t}\n}",
		`"todo.add": model.Add;`,
		`"todo.ext": dep.Ext;`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("commands.ts missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "note:") {
		t.Fatalf("nothing should fall back:\n%s", got)
	}
}

func TestLoadErrorsOnlyFailPackagesWithOurDeclarations(t *testing.T) {
	files := func() map[string]string {
		return map[string]string{"broken/broken.go": "package broken\n\nvar X int = \"s\"\n"}
	}
	if err := bindApp(t, "x.none", files()); err == nil || strings.Contains(err.Error(), "broken") {
		t.Fatalf("an unrelated broken package must not fail the run: %v", err)
	}
	f := files()
	f["cells/cells.go"] = "package cells\n\nimport \"github.com/nkapatos/templatic/live\"\n\nvar _ = live.NewCell\n\nvar Y int = \"s\"\n"
	err := bindApp(t, "x.none", f)
	if err == nil || !strings.Contains(err.Error(), "package example.test/app/cells") {
		t.Fatalf("errors in a package using live must be reported: %v", err)
	}
}

func TestCommandInputJSONv2CannotDecodeFails(t *testing.T) {
	dir := t.TempDir()
	testapp.WriteModule(t, dir, "example.test/app")
	writeFiles(t, dir, map[string]string{
		"todo/todo.go": `package todo

import (
	"time"

	"github.com/nkapatos/templatic/live"
)

type Snooze struct {
	For time.Duration ` + "`json:\"for\"`" + `
}

var S = live.NewCommand[Snooze]("todo.snooze")
`,
	})
	err := writeCommandsTS(Options{Root: dir}, mustLoad(t, dir))
	if err == nil || !strings.Contains(err.Error(), "todo.snooze") || !strings.Contains(err.Error(), "field for: time.Duration") {
		t.Fatalf("want a build error naming the command and field, got %v", err)
	}
}
