package cli

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nkapatos/templatic/generate"
	"github.com/nkapatos/templatic/internal/testapp"
)

func copyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("testdata/app")); err != nil {
		t.Fatal(err)
	}
	testapp.WriteModule(t, root, "testdata.example/templatic-app")
	return root
}

func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

type pluginFn struct {
	name  string
	setup func(*generate.Builder) error
}

func (p pluginFn) Name() string                    { return p.name }
func (p pluginFn) Setup(b *generate.Builder) error { return p.setup(b) }

func TestHelpGoesToStdout(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}, {"generate", "--help"}, {"dev", "-h"}, {"init", "--help"}} {
		var out, errb bytes.Buffer
		if c := Main(args, &out, &errb); c != 0 || errb.Len() != 0 || !strings.Contains(out.String(), "USAGE:") {
			t.Errorf("%v: code %d, stdout %q, stderr %q", args, c, out.String(), errb.String())
		}
	}
	var out bytes.Buffer
	Main([]string{"generate", "--help"}, &out, &bytes.Buffer{})
	for _, want := range []string{"--root", "--client", "--define", "--loader"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("generate help lacks %s:\n%s", want, out.String())
		}
	}
}

func TestRunExitCodes(t *testing.T) {
	var out, errb bytes.Buffer
	if c := Main([]string{"generate", "-nope"}, &out, &errb); c != 2 {
		t.Fatalf("bad flag: %d", c)
	}
	errb.Reset()
	if c := Main([]string{"generate", "-define", "x"}, &out, &errb); c != 1 || !strings.Contains(errb.String(), "--define wants KEY=VALUE") {
		t.Fatalf("bad define: %d %q", c, errb.String())
	}
}

func TestFlagSpellings(t *testing.T) {
	for _, args := range [][]string{
		{"generate", "-root", "", "-skip-bundle"},
		{"generate", "--root=", "--skip-bundle"},
		{"-root", "", "generate", "-skip-bundle"},
	} {
		root := copyFixture(t)
		for i, a := range args {
			if a == "" || a == "--root=" {
				args[i] += root
			}
		}
		var out, errb bytes.Buffer
		if c := Main(args, &out, &errb); c != 0 {
			t.Errorf("%v: exit %d: %s", args, c, errb.String())
		}
	}
}

func TestDefineKeepsCommas(t *testing.T) {
	var got map[string]string
	var out, errb bytes.Buffer
	c := Main([]string{"generate", "-root", copyFixture(t), "-skip-bundle", "-define", `A={"a":1,"b":2}`}, &out, &errb,
		Options(func(o *generate.Options) { got = o.Defines }))
	if c != 0 || got["A"] != `{"a":1,"b":2}` {
		t.Fatalf("exit %d: %v %s", c, got, errb.String())
	}
}

func TestGenerateParityWithAPI(t *testing.T) {
	viaCLI, viaAPI := copyFixture(t), copyFixture(t)
	var out, errb bytes.Buffer
	if c := Main([]string{"generate", "-root", viaCLI}, &out, &errb); c != 0 {
		t.Fatalf("exit %d: %s", c, errb.String())
	}
	if _, err := generate.Run(generate.Options{Root: viaAPI}); err != nil {
		t.Fatal(err)
	}
	a, b := tree(t, viaCLI), tree(t, viaAPI)
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("file sets differ: %d vs %d", len(a), len(b))
	}
	for k, v := range a {
		if b[k] != v {
			t.Errorf("%s differs between CLI and API", k)
		}
	}
	if !strings.HasPrefix(out.String(), "templ:  ") || !strings.Contains(out.String(), "(+ tc.json)") {
		t.Fatalf("summary output: %q", out.String())
	}
}

func TestWithAndOptionsReachGenerate(t *testing.T) {
	root := copyFixture(t)
	var order []string
	p := pluginFn{"probe", func(b *generate.Builder) error {
		b.OnConfig(func(o *generate.Options) error { order = append(order, "plugin:"+o.ClientDir); return nil })
		return nil
	}}
	var out, errb bytes.Buffer
	c := Main([]string{"generate", "-root", root, "-skip-bundle"}, &out, &errb,
		With(p),
		Options(func(o *generate.Options) { order = append(order, "options") }))
	if c != 0 {
		t.Fatalf("exit %d: %s", c, errb.String())
	}
	if strings.Join(order, ",") != "options,plugin:client" {
		t.Fatalf("options run after flags, before hooks: %v", order)
	}
	errb.Reset()
	bad := pluginFn{"bad", func(*generate.Builder) error { return os.ErrInvalid }}
	if c := Main([]string{"generate", "-root", root, "-skip-bundle"}, &out, &errb, With(bad)); c != 1 || !strings.Contains(errb.String(), "plugin bad:") {
		t.Fatalf("plugin error: %d %q", c, errb.String())
	}
}

func TestWithReachesDev(t *testing.T) {
	root := copyFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen := make(chan bool, 1)
	p := pluginFn{"probe", func(b *generate.Builder) error {
		b.OnConfig(func(o *generate.Options) error {
			select {
			case seen <- o.Mode == generate.ModeDev:
			default:
			}
			return nil
		})
		return nil
	}}
	done := make(chan int)
	go func() {
		done <- run(ctx, []string{"dev", "-root", root, "-skip-bundle"}, &bytes.Buffer{}, &bytes.Buffer{}, []Option{With(p)})
	}()
	select {
	case dev := <-seen:
		if !dev {
			t.Fatal("dev did not build in dev mode")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the plugin never reached dev's build")
	}
	cancel()
	if c := <-done; c != 0 {
		t.Fatalf("dev exit %d", c)
	}
}
