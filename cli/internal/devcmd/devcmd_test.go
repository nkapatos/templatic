package devcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nkapatos/templatic/generate"
	"github.com/nkapatos/templatic/internal/buildfiles"
	"github.com/nkapatos/templatic/internal/testapp"
)

type devHarness struct {
	t      *testing.T
	root   string
	builds chan error
	cancel context.CancelFunc
	done   chan struct{}
}

func startDev(t *testing.T, watch ...string) *devHarness {
	return startDevOut(t, &bytes.Buffer{}, watch...)
}

func startDevOut(t *testing.T, out *bytes.Buffer, watch ...string) *devHarness {
	t.Helper()
	root := copyFixture(t)
	h := &devHarness{t: t, root: root, builds: make(chan error, 16), done: make(chan struct{})}
	var mu sync.Mutex
	watcher := pluginFn{name: "watch", setup: func(b *generate.Builder) error { b.Watch(watch...); return nil }}
	d := &devLoop{
		opts: generate.Options{Root: root, SkipBundle: true, Mode: generate.ModeDev, Plugins: []generate.Plugin{watcher}},
		root: root, client: "client", dist: ".templatic/dist",
		// wide enough that a burst written at once lands in one window on a slow runner
		debounce: 100 * time.Millisecond,
		stdout:   out, stderr: out,
	}
	d.onBuild = func(err error) { mu.Lock(); defer mu.Unlock(); h.builds <- err }
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() { d.run(ctx); close(h.done) }()
	t.Cleanup(func() { cancel(); <-h.done })
	if err := h.next(); err != nil {
		t.Fatalf("first build: %v", err)
	}
	return h
}

func (h *devHarness) next() error {
	h.t.Helper()
	select {
	case err := <-h.builds:
		return err
	case <-time.After(5 * time.Second):
		h.t.Fatal("no build")
		return nil
	}
}

func (h *devHarness) none(why string) {
	h.t.Helper()
	select {
	case err := <-h.builds:
		h.t.Fatalf("%s: unexpected build (%v)", why, err)
	// longer than the debounce plus a build, so a stray build lands here rather than in the next step
	case <-time.After(400 * time.Millisecond):
	}
}

func (h *devHarness) write(rel, body string) {
	h.t.Helper()
	p := filepath.Join(h.root, rel)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func TestDevLoopRebuildsOnSourcesOnly(t *testing.T) {
	h := startDev(t, "content/**/*.md")
	h.none("idle after first build (the pass's own outputs must not retrigger)")

	h.write("client/app.ts", "export {};\n")
	if err := h.next(); err != nil {
		t.Fatal(err)
	}
	h.none("one build per change")

	for i := 0; i < 5; i++ {
		h.write("client/modules/m.ts", "export const n = "+string(rune('0'+i))+";\n")
	}
	if err := h.next(); err != nil {
		t.Fatal(err)
	}
	h.none("burst debounced")

	h.write(".templatic/runtime/x.ts", "//\n")
	h.write(".templatic/dist/x.js", "//\n")
	h.none("generated/dist writes")

	h.write("content/a/b.md", "# hi\n")
	if err := h.next(); err != nil {
		t.Fatal(err)
	}
}

func TestDevLoopReportsBuildErrors(t *testing.T) {
	var out bytes.Buffer
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(out.String())
		}
	})
	h := startDevOut(t, &out)
	card := filepath.Join(h.root, "widgets", "card.templ")
	good, err := os.ReadFile(card)
	if err != nil {
		t.Fatal(err)
	}
	h.write("widgets/card.templ", string(good)+"\ntempl Broken( {\n")
	if err := h.next(); err == nil {
		t.Fatal("broken templ built")
	}
	errFile := filepath.Join(h.root, ".templatic", "dist", buildfiles.DevError)
	if b, err := os.ReadFile(errFile); err != nil || !bytes.Contains(b, []byte(`"message"`)) {
		t.Fatalf("error file: %q %v", b, err)
	}
	h.write("widgets/card.templ", string(good))
	if err := h.next(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(errFile); !os.IsNotExist(err) {
		t.Fatal("error file survived a good build")
	}
}

func TestGlobRE(t *testing.T) {
	for _, c := range []struct {
		glob, path string
		want       bool
	}{
		{"content/**/*.md", "content/a/b.md", true},
		{"content/**/*.md", "content/b.md", true},
		{"content/*.md", "content/a/b.md", false},
		{"styles/**", "styles/x/y.css", true},
		{"a?.txt", "ab.txt", true},
	} {
		if got := globRE(c.glob).MatchString(c.path); got != c.want {
			t.Errorf("%s ~ %s = %v", c.glob, c.path, got)
		}
	}
}

func TestDevExecRestartsOnGoChange(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes")
	}
	root := copyFixture(t)
	log := filepath.Join(root, "starts.log")
	d := &devLoop{
		opts: generate.Options{Root: root, SkipBundle: true, Mode: generate.ModeDev},
		root: root, client: "client", dist: ".templatic/dist",
		exec:     "echo $$ >> starts.log; exec sleep 30",
		debounce: 30 * time.Millisecond,
		stdout:   &bytes.Buffer{}, stderr: &bytes.Buffer{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { d.run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	pids := func() []string {
		b, _ := os.ReadFile(log)
		return strings.Fields(string(b))
	}
	starts := func() int { return len(pids()) }
	waitFor := func(n int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for starts() < n {
			if time.Now().After(deadline) {
				t.Fatalf("starts = %d, want %d", starts(), n)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor(1)
	if err := os.WriteFile(filepath.Join(root, "widgets", "extra.go"), []byte("package widgets\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(2)

	cancel()
	<-done
	for _, p := range pids() {
		pid, _ := strconv.Atoi(p)
		if proc, err := os.FindProcess(pid); err == nil && proc.Signal(syscall.Signal(0)) == nil {
			t.Errorf("app process %d still runs after dev stopped", pid)
		}
	}
}

func copyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../testdata/app")); err != nil {
		t.Fatal(err)
	}
	testapp.WriteModule(t, root, "testdata.example/templatic-app")
	return root
}

type pluginFn struct {
	name  string
	setup func(*generate.Builder) error
}

func (p pluginFn) Name() string                    { return p.name }
func (p pluginFn) Setup(b *generate.Builder) error { return p.setup(b) }
