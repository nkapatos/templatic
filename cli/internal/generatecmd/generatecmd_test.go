package generatecmd

import (
	"context"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/nkapatos/templatic/generate"
	"github.com/urfave/cli/v3"
)

func parse(t *testing.T, args ...string) (generate.Options, error) {
	t.Helper()
	var opts generate.Options
	var assembleErr error
	root := &cli.Command{
		Name:  "t",
		Flags: []cli.Flag{&cli.StringFlag{Name: "root", Value: "."}},
		Commands: []*cli.Command{{
			Name:  "generate",
			Flags: Flags("prod"),
			Action: func(_ context.Context, cmd *cli.Command) error {
				opts, assembleErr = Assemble(cmd, Config{})
				return nil
			},
		}},
	}
	if err := root.Run(context.Background(), append([]string{"t", "generate"}, args...)); err != nil {
		t.Fatal(err)
	}
	return opts, assembleErr
}

func TestFlagParsing(t *testing.T) {
	o, err := parse(t, "-root", "x", "-mode", "dev", "-define", "A=1", "-define", `B="s=t"`,
		"-alias", "react=preact/compat", "-loader", ".svg=file", "-loader", ".md=text", "-loader", ".bin=dataurl")
	if err != nil {
		t.Fatal(err)
	}
	if o.Root != "x" || o.Mode != generate.ModeDev {
		t.Fatalf("basics: %+v", o)
	}
	if o.Defines["A"] != "1" || o.Defines["B"] != `"s=t"` {
		t.Fatalf("defines: %v", o.Defines)
	}
	if o.Alias["react"] != "preact/compat" {
		t.Fatalf("alias: %v", o.Alias)
	}
	if o.Loader[".svg"] != api.LoaderFile || o.Loader[".md"] != api.LoaderText || o.Loader[".bin"] != api.LoaderDataURL {
		t.Fatalf("loaders: %v", o.Loader)
	}
	d, err := parse(t)
	if err != nil || d.Mode != generate.ModeProd || d.Alias != nil || d.Loader != nil || d.DistDir != generate.DefaultDistDir {
		t.Fatalf("defaults: %+v %v", d, err)
	}
}

func TestFlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-mode", "staging"},
		{"-define", "NOEQUALS"},
		{"-alias", "a="},
		{"-alias", "=b"},
		{"-loader", "svg=file"},
		{"-loader", ".svg=wasm"},
		{"-loader", ".svg"},
	} {
		if _, err := parse(t, args...); err == nil {
			t.Errorf("%v must be rejected", args)
		}
	}
}

func TestConfigRunsAfterFlags(t *testing.T) {
	var opts generate.Options
	root := &cli.Command{
		Name:  "t",
		Flags: append(Flags("prod"), &cli.StringFlag{Name: "root", Value: "."}),
		Action: func(_ context.Context, cmd *cli.Command) error {
			var err error
			opts, err = Assemble(cmd, Config{Mutate: []func(*generate.Options){func(o *generate.Options) { o.ClientDir += "!" }}})
			return err
		},
	}
	if err := root.Run(context.Background(), []string{"t", "-client", "web"}); err != nil {
		t.Fatal(err)
	}
	if opts.ClientDir != "web!" {
		t.Fatalf("ClientDir = %q", opts.ClientDir)
	}
}
