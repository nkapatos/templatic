// Package checktscmd implements `templatic check-ts`.
package checktscmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nkapatos/templatic/generate"
	"github.com/urfave/cli/v3"
)

// Command returns `templatic check-ts`, which runs tsc --noEmit over the client.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "check-ts",
		Usage: "run tsc --noEmit over the client",
		Action: func(_ context.Context, cmd *cli.Command) error {
			root := cmd.String("root")
			generated := filepath.ToSlash(filepath.Join(generate.GenDir, "tsconfig.json"))
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(generated))); err != nil {
				return fmt.Errorf("check-ts: %s not found (run templatic generate first)", generated)
			}
			cfgPath := generated
			if _, err := os.Stat(filepath.Join(root, "tsconfig.json")); err == nil {
				cfgPath = "tsconfig.json"
			}
			tsc := exec.Command("npx", "--no-install", "tsc", "--noEmit", "-p", cfgPath)
			tsc.Dir = root
			tsc.Stdout, tsc.Stderr = cmd.Root().Writer, cmd.Root().ErrWriter
			if err := tsc.Run(); err != nil {
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					return cli.Exit("", 1)
				}
				return fmt.Errorf("check-ts: %w", err)
			}
			return nil
		},
	}
}
