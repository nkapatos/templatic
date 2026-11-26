// Package generatecmd implements `templatic generate` and the flags it shares with `templatic dev`.
package generatecmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/nkapatos/templatic/generate"
	"github.com/urfave/cli/v3"
)

// Config is what an app's build program adds to the flag-derived options.
type Config struct {
	// Plugins are registered after the flags are applied.
	Plugins []generate.Plugin
	// Mutate funcs run last, so code has the last word over flags.
	Mutate []func(*generate.Options)
}

// Command returns `templatic generate`.
func Command(cfg Config) *cli.Command {
	return &cli.Command{
		Name:        "generate",
		Usage:       "render Go glue for .templ files, generate island TypeScript, copy the client runtime and bundle",
		UsageText:   "templatic generate [flags]",
		Description: "Everything generate writes besides the Go glue lives in .templatic/; `templatic clean` removes it.",
		Flags:       Flags("prod"),
		Action: func(_ context.Context, cmd *cli.Command) error {
			opts, err := Assemble(cmd, cfg)
			if err != nil {
				return err
			}
			summary, err := generate.Run(opts)
			if err != nil {
				return err
			}
			printSummary(cmd.Root().Writer, summary, opts.DistDir)
			return nil
		},
	}
}

// Flags returns the flags of generate and dev; mode is the default of --mode. Root is the root command's flag.
func Flags(mode string) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "client", Value: "client", Usage: "the app's client code: app.ts, modules/, islands/{tag}.ts"},
		&cli.StringFlag{Name: "dist", Value: generate.DefaultDistDir, Usage: "bundle output directory"},
		&cli.BoolFlag{Name: "skip-bundle", Usage: "skip the esbuild step"},
		&cli.StringFlag{Name: "mode", Value: mode, Usage: "build mode: prod (minified) or dev (unminified, linked sourcemaps, __TC_DEV__)"},
		&cli.StringSliceFlag{Name: "define", Usage: "esbuild compile-time define, repeatable (`KEY=VALUE`)"},
		&cli.StringSliceFlag{Name: "alias", Usage: "esbuild import alias, repeatable (`FROM=TO`)"},
		&cli.StringSliceFlag{Name: "loader", Usage: "loader for an extension, repeatable (`.ext=file|dataurl|text`)"},
	}
}

var loaders = map[string]api.Loader{
	"file":    api.LoaderFile,
	"dataurl": api.LoaderDataURL,
	"text":    api.LoaderText,
}

// Assemble builds the generate options from the parsed flags of cmd, then adds cfg.
func Assemble(cmd *cli.Command, cfg Config) (generate.Options, error) {
	o := generate.Options{
		Root:       cmd.String("root"),
		ClientDir:  cmd.String("client"),
		DistDir:    cmd.String("dist"),
		SkipBundle: cmd.Bool("skip-bundle"),
	}
	switch mode := cmd.String("mode"); mode {
	case "prod":
		o.Mode = generate.ModeProd
	case "dev":
		o.Mode = generate.ModeDev
	default:
		return o, fmt.Errorf("--mode wants prod or dev, got %q", mode)
	}
	o.Defines = map[string]string{}
	for _, d := range cmd.StringSlice("define") {
		k, v, ok := strings.Cut(d, "=")
		if !ok || k == "" {
			return o, fmt.Errorf("--define wants KEY=VALUE, got %q", d)
		}
		o.Defines[k] = v
	}
	if aliases := cmd.StringSlice("alias"); len(aliases) > 0 {
		o.Alias = map[string]string{}
		for _, a := range aliases {
			k, v, ok := strings.Cut(a, "=")
			if !ok || k == "" || v == "" {
				return o, fmt.Errorf("--alias wants FROM=TO, got %q", a)
			}
			o.Alias[k] = v
		}
	}
	if ls := cmd.StringSlice("loader"); len(ls) > 0 {
		o.Loader = map[string]api.Loader{}
		for _, l := range ls {
			ext, name, ok := strings.Cut(l, "=")
			if !ok || !strings.HasPrefix(ext, ".") || len(ext) < 2 {
				return o, fmt.Errorf("--loader wants .ext=file|dataurl|text, got %q", l)
			}
			ld, ok := loaders[name]
			if !ok {
				return o, fmt.Errorf("--loader %s: unknown loader %q (want file, dataurl or text)", ext, name)
			}
			o.Loader[ext] = ld
		}
	}
	o.Plugins = append(o.Plugins, cfg.Plugins...)
	for _, m := range cfg.Mutate {
		m(&o)
	}
	return o, nil
}

func printSummary(stdout io.Writer, summary *generate.Summary, dist string) {
	fmt.Fprintf(stdout, "templ:  %d file(s) → *_templ.go\n", len(summary.Emitted))
	for _, isl := range summary.Islands {
		variant := isl.Variant
		if variant == "" {
			variant = "hooks"
		}
		fmt.Fprintf(stdout, "island: %-24s props=%s variant=%s\n", isl.Tag, isl.PropsType, variant)
	}
	fmt.Fprintf(stdout, "runtime: %d file(s) → %s/runtime\n", len(summary.RuntimeFiles), generate.GenDir)
	if summary.Manifest != "" {
		fmt.Fprintf(stdout, "bundle: %d output(s) → %s (+ tc.json)\n", len(summary.Bundle), dist)
		for _, out := range summary.Bundle {
			fmt.Fprintf(stdout, "    %s\n", out)
		}
	}
}
