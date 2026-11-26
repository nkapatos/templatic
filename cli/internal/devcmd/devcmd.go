// Package devcmd implements `templatic dev`.
package devcmd

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nkapatos/templatic/cli/internal/generatecmd"
	"github.com/nkapatos/templatic/generate"
	"github.com/nkapatos/templatic/internal/buildfiles"
	"github.com/urfave/cli/v3"
)

// Command returns `templatic dev`: it reruns generate on changes and restarts --exec when .go files change.
func Command(cfg generatecmd.Config) *cli.Command {
	return &cli.Command{
		Name:  "dev",
		Usage: "rerun generate on changes; run and restart the app",
		Flags: append(generatecmd.Flags("dev"),
			&cli.StringFlag{Name: "exec", Usage: `run this shell command as the app and restart it when .go files change (e.g. "go run .")`},
			&cli.DurationFlag{Name: "debounce", Value: 100 * time.Millisecond, Usage: "quiet time after the last change before rebuilding"},
		),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			opts, err := generatecmd.Assemble(cmd, cfg)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			d := &devLoop{
				opts: opts, client: opts.ClientDir, dist: opts.DistDir, root: opts.Root,
				exec: cmd.String("exec"), debounce: cmd.Duration("debounce"),
				stdout: cmd.Root().Writer, stderr: cmd.Root().ErrWriter,
			}
			if code := d.run(ctx); code != 0 {
				return cli.Exit("", code)
			}
			return nil
		},
	}
}

type devLoop struct {
	opts               generate.Options
	root, client, dist string
	exec               string
	debounce           time.Duration
	stdout, stderr     io.Writer
	globs              []*regexp.Regexp
	app                *appProc
	builds             int
	onBuild            func(err error)
	seen               map[string]string
}

// pending is what the changes since the last quiet period call for.
type pending struct{ build, restart bool }

func (d *devLoop) run(ctx context.Context) int {
	// The app child and the loop share the output streams.
	var mu sync.Mutex
	d.stdout = &lockedWriter{mu: &mu, w: d.stdout}
	d.stderr = &lockedWriter{mu: &mu, w: d.stderr}
	d.seen = map[string]string{}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(d.stderr, "dev: %v\n", err)
		return 1
	}
	defer w.Close()
	// Watch before building, so an edit that lands mid-build still counts.
	var p pending
	d.addTree(w, d.root, &p)
	summary := d.build()
	d.absorb(summary.emitted, summary.islands)
	p = pending{}
	if d.exec != "" {
		d.app = startApp(d.exec, d.root, d.stdout, d.stderr)
		defer d.app.stop()
	}
	fmt.Fprintf(d.stdout, "dev: watching %s (ctrl-c to stop)\n", d.root)
	quiet := time.NewTimer(time.Hour)
	quiet.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0
		case err := <-w.Errors:
			fmt.Fprintf(d.stderr, "dev: watch: %v\n", err)
		case ev := <-w.Events:
			if d.classify(w, ev, &p) {
				quiet.Reset(d.debounce)
			}
		case <-quiet.C:
			if p.build {
				d.absorb(nil, d.build().islands)
			}
			if p.restart && d.app != nil {
				fmt.Fprintln(d.stdout, "dev: .go changed, restarting app")
				d.app.stop()
				d.app = startApp(d.exec, d.root, d.stdout, d.stderr)
			}
			p = pending{}
		}
	}
}

// classify records what ev calls for in p and reports whether it called for anything.
func (d *devLoop) classify(w *fsnotify.Watcher, ev fsnotify.Event, p *pending) bool {
	if ev.Op == fsnotify.Chmod {
		return false
	}
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			return d.addTree(w, ev.Name, p)
		}
	}
	return d.mark(ev.Name, p)
}

// mark records what a change to the file at abs calls for in p, unless the file is hidden (the temp files of atomic writes) or as the dev loop last saw it.
func (d *devLoop) mark(abs string, p *pending) bool {
	rel, err := filepath.Rel(d.root, abs)
	if err != nil || strings.HasPrefix(filepath.Base(abs), ".") {
		return false
	}
	rel = filepath.ToSlash(rel)
	stamp := stampOf(abs)
	if old, ok := d.seen[rel]; ok && old == stamp {
		return false
	}
	d.seen[rel] = stamp
	switch {
	case strings.HasSuffix(rel, ".go"):
		p.restart = true
	case strings.HasSuffix(rel, ".templ"), strings.HasPrefix(rel, filepath.ToSlash(filepath.Clean(d.client))+"/"):
		p.build = true
	default:
		for _, g := range d.globs {
			if g.MatchString(rel) {
				p.build = true
				break
			}
		}
	}
	return p.build || p.restart
}

// addTree watches dir and the directories below it that the loop cares about, and marks the files found there.
func (d *devLoop) addTree(w *fsnotify.Watcher, dir string, p *pending) bool {
	dist := filepath.ToSlash(filepath.Clean(d.dist))
	var changed bool
	_ = filepath.WalkDir(dir, func(abs string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(d.root, abs)
		rel = filepath.ToSlash(rel)
		if !e.IsDir() {
			if dir != d.root && d.mark(abs, p) {
				changed = true
			}
			return nil
		}
		if rel != "." && (strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" || e.Name() == "vendor" || rel == dist) {
			return filepath.SkipDir
		}
		if err := w.Add(abs); err != nil {
			fmt.Fprintf(d.stderr, "dev: watch %s: %v\n", rel, err)
		}
		return nil
	})
	return changed
}

// absorb records the current state of the files a build itself wrote, so they do not count as changes.
func (d *devLoop) absorb(emitted, islands []string) {
	for _, rel := range append(emitted, islands...) {
		d.seen[rel] = stampOf(filepath.Join(d.root, filepath.FromSlash(rel)))
	}
}

func stampOf(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d/%d", info.ModTime().UnixNano(), info.Size())
}

type buildResult struct{ emitted, islands []string }

func (d *devLoop) build() buildResult {
	start := time.Now()
	summary, err := generate.Run(d.opts)
	errPath := filepath.Join(d.root, d.dist, buildfiles.DevError)
	var res buildResult
	if err != nil {
		fmt.Fprintf(d.stderr, "dev: build failed: %v\n", err)
		body, _ := json.Marshal(struct {
			Message string `json:"message"`
		}{err.Error()}, jsontext.AllowInvalidUTF8(true))
		// Only into a dist dir generate recognized as its own.
		if _, err := os.Stat(filepath.Join(d.root, d.dist, buildfiles.Generations)); err == nil {
			_ = os.WriteFile(errPath, body, 0o644)
		}
	} else {
		_ = os.Remove(errPath)
		d.globs = d.globs[:0]
		for _, g := range summary.Watch {
			d.globs = append(d.globs, globRE(g))
		}
		res.emitted = summary.Emitted
		for _, isl := range summary.Islands {
			res.islands = append(res.islands, path.Join(filepath.ToSlash(filepath.Clean(d.client)), "islands", isl.Tag+".ts"))
		}
		fmt.Fprintf(d.stdout, "dev: built in %s (%d templ, %d island(s), %d output(s))\n",
			time.Since(start).Round(time.Millisecond), len(summary.Emitted), len(summary.Islands), len(summary.Bundle))
	}
	d.builds++
	if d.onBuild != nil {
		d.onBuild(err)
	}
	return res
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func globRE(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	g := filepath.ToSlash(glob)
	for i := 0; i < len(g); i++ {
		switch c := g[i]; {
		case c == '*' && i+1 < len(g) && g[i+1] == '*':
			i++
			if i+1 < len(g) && g[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?")
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// appProc runs in its own process group so `go run`'s compiled child dies with it.
type appProc struct {
	cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
}

func startApp(command, dir string, stdout, stderr io.Writer) *appProc {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = stdout, stderr
	setProcessGroup(cmd)
	p := &appProc{cmd: cmd, done: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "dev: -exec %q: %v\n", command, err)
		close(p.done)
		return p
	}
	go func() { _ = cmd.Wait(); close(p.done) }()
	return p
}

func (p *appProc) stop() {
	p.once.Do(func() {
		if p.cmd.Process == nil {
			return
		}
		select {
		case <-p.done:
			return
		default:
		}
		signalGroup(p.cmd, syscall.SIGTERM)
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
			signalGroup(p.cmd, syscall.SIGKILL)
			<-p.done
		}
	})
}
