package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

func distOf(o Options) string { return filepath.Join(o.Root, ".templatic", "dist") }

func TestDevModeUnminifiedWithLinkedSourcemaps(t *testing.T) {
	prod := testAppOpts(t)
	if _, err := Run(prod); err != nil {
		t.Fatal(err)
	}
	dev := testAppOpts(t)
	dev.Mode = ModeDev
	setIslandScript(t, dev, `onMount((el) => { el.dataset.dev = String(__TC_DEV__) + process.env.NODE_ENV; })`)
	if _, err := Run(dev); err != nil {
		t.Fatal(err)
	}
	dist := distOf(dev)
	boot := mustBootName(t, dev)
	src := read(t, filepath.Join(dist, boot))
	if !strings.Contains(src, "sourceMappingURL="+boot+".map") {
		t.Fatalf("hashed boot must link its renamed map; tail: %q", src[max(0, len(src)-120):])
	}
	if _, err := os.Stat(filepath.Join(dist, boot+".map")); err != nil {
		t.Fatalf("boot sourcemap missing: %v", err)
	}
	chunk := mustIslandChunk(t, dev, "user-card")
	if !strings.Contains(chunk, "development") || strings.Contains(chunk, "__TC_DEV__") {
		t.Fatalf("dev defines not applied: %s", chunk[:min(len(chunk), 400)])
	}
	if len(src) <= len(read(t, filepath.Join(distOf(prod), mustBootName(t, prod)))) {
		t.Fatal("dev boot should be larger (unminified) than prod boot")
	}
	_ = filepath.WalkDir(dist, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") || filepath.Base(p) == "boot.js" {
			return nil
		}
		if _, err := os.Stat(p + ".map"); err != nil {
			t.Errorf("%s has no sourcemap beside it", p)
		}
		if !strings.Contains(read(t, p), "sourceMappingURL="+filepath.Base(p)+".map") {
			t.Errorf("%s does not link its own map", p)
		}
		return nil
	})
}

func TestProdModeHasNoSourcemaps(t *testing.T) {
	opts := testAppOpts(t)
	opts.Mode = ModeProd
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	_ = filepath.WalkDir(distOf(opts), func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".map") {
			t.Errorf("prod emitted %s", p)
		}
		return nil
	})
}

func TestUnknownModeRejected(t *testing.T) {
	opts := testAppOpts(t)
	opts.Mode = "staging"
	if _, err := Run(opts); err == nil {
		t.Fatal("unknown mode must fail")
	}
}

func TestLoaderAndAliasOptionsReachBuild(t *testing.T) {
	opts := testAppOpts(t)
	opts.Loader = map[string]api.Loader{".txt": api.LoaderText}
	opts.Alias = map[string]string{"greeting-alias": "./client/greeting.txt"}
	if err := os.WriteFile(filepath.Join(opts.Root, "client", "greeting.txt"), []byte("hello-from-text-loader"), 0o644); err != nil {
		t.Fatal(err)
	}
	setIslandScript(t, opts, `import g from "greeting-alias";
	onMount((el) => { el.dataset.g = g; })`)
	if _, err := Run(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mustIslandChunk(t, opts, "user-card"), "hello-from-text-loader") {
		t.Fatal("alias + text loader did not reach the build")
	}
}
