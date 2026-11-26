package generate

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFiles writes rel-path to source pairs under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mustLoad(t *testing.T, dir string) *program {
	t.Helper()
	prog, err := loadProgram(dir)
	if err != nil {
		t.Fatal(err)
	}
	return prog
}
