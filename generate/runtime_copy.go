package generate

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/nkapatos/templatic/internal/clientruntime"
)

func copyRuntime(opts Options, s *Summary) error {
	files, err := clientruntime.Files()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("generate: runtime sources missing from the templatic module (clientruntime embeds none)")
	}
	dir := filepath.Join(opts.genDir(), "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		content := files[name]
		target := filepath.Join(dir, name)
		if err := writeAtomic(target, content); err != nil {
			return err
		}
		s.RuntimeFiles = append(s.RuntimeFiles, filepath.ToSlash(GenDir+"/runtime/"+name))
	}
	tsconfig := filepath.Join(opts.genDir(), "tsconfig.json")
	return writeAtomic(tsconfig, []byte(tsconfigBody))
}

const tsconfigBody = `{
	"compilerOptions": {
		"paths": {
			"templatic": ["./runtime/index.ts"],
			"templatic/commands": ["./commands.ts"],
			"templatic/islands/*": ["./islands/*"]
		}
	}
}
`
