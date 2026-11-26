package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/nkapatos/templatic/cli/internal/checktscmd"
	"github.com/nkapatos/templatic/cli/internal/cleancmd"
	"github.com/nkapatos/templatic/cli/internal/devcmd"
	"github.com/nkapatos/templatic/cli/internal/generatecmd"
	"github.com/nkapatos/templatic/cli/internal/initcmd"
	"github.com/nkapatos/templatic/generate"
	urfave "github.com/urfave/cli/v3"
)

// Option customises the CLI for an app-owned build program.
type Option func(*config)

type config struct {
	plugins []generate.Plugin
	mutate  []func(*generate.Options)
}

// With registers templatic plugins for generate and dev, run after the flag-derived options.
func With(plugins ...generate.Plugin) Option {
	return func(c *config) { c.plugins = append(c.plugins, plugins...) }
}

// Options mutates the generate options of generate and dev after the flags, so code has the last word.
func Options(fn func(*generate.Options)) Option {
	return func(c *config) { c.mutate = append(c.mutate, fn) }
}

// Main runs the CLI with args (without the program name) and returns the exit code: 0 on success, 2 for a usage error, 1 otherwise.
func Main(args []string, stdout, stderr io.Writer, opts ...Option) int {
	return run(context.Background(), args, stdout, stderr, opts)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, opts []Option) int {
	var c config
	for _, o := range opts {
		o(&c)
	}
	cfg := generatecmd.Config{Plugins: c.plugins, Mutate: c.mutate}
	root := &urfave.Command{
		Name:           "templatic",
		Usage:          "typed islands and live state on net/http and templ",
		Writer:         stdout,
		ErrWriter:      stderr,
		ExitErrHandler: func(context.Context, *urfave.Command, error) {},
		Flags:          []urfave.Flag{&urfave.StringFlag{Name: "root", Value: ".", Usage: "module root"}},
		Commands: []*urfave.Command{
			generatecmd.Command(cfg),
			devcmd.Command(cfg),
			initcmd.Command(),
			cleancmd.Command(),
			checktscmd.Command(),
		},
		Action: func(_ context.Context, cmd *urfave.Command) error {
			if cmd.Args().Present() {
				return urfave.Exit(fmt.Errorf("unknown command %q (see `templatic help`)", cmd.Args().First()), 2)
			}
			cmd.Writer = stderr
			_ = urfave.ShowRootCommandHelp(cmd)
			return urfave.Exit("", 2)
		},
	}
	_ = root.Walk(func(c *urfave.Command) error {
		c.DisableSliceFlagSeparator = true
		c.OnUsageError = func(_ context.Context, _ *urfave.Command, err error, _ bool) error {
			return urfave.Exit(err, 2)
		}
		return nil
	})
	err := root.Run(ctx, append([]string{root.Name}, args...))
	if err == nil {
		return 0
	}
	if err.Error() != "" {
		fmt.Fprintln(stderr, "templatic:", err)
	}
	var coder urfave.ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}
