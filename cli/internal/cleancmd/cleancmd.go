// Package cleancmd implements `templatic clean`.
package cleancmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nkapatos/templatic/generate"
	"github.com/urfave/cli/v3"
)

// Command returns `templatic clean`, which removes .templatic only when templatic made it.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "clean",
		Usage: "remove .templatic/",
		Action: func(_ context.Context, cmd *cli.Command) error {
			root, stdout := cmd.String("root"), cmd.Root().Writer
			dir := filepath.Join(root, generate.GenDir)
			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(stdout, "clean: no %s under %s\n", generate.GenDir, root)
				return nil
			}
			if !generate.HasMarker(root) {
				return fmt.Errorf("clean: %s has no %s marker and was not created by templatic: not removed", dir, generate.MarkerFile)
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("clean: %w", err)
			}
			fmt.Fprintf(stdout, "clean: removed %s\n", dir)
			return nil
		},
	}
}
