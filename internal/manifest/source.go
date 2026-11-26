package manifest

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nkapatos/templatic/internal/buildfiles"
	"github.com/nkapatos/templatic/live"
)

// Source hands layouts the current manifest; in dev it re-reads tc.json on
// rebuilds and, once Notify names a World, tells open pages.
type Source struct {
	dir string
	dev bool

	mu      sync.Mutex
	cur     *Manifest
	stamp   string
	errMsg  string
	worlds  []*live.World
	started bool
	stop    chan struct{}
}

// For loads the manifest in distDir; mode "dev" watches it and tolerates a
// missing tc.json, anything else reads it once.
func For(mode, distDir string) (*Source, error) {
	s := &Source{dir: distDir, dev: mode == "dev"}
	m, err := Load(distDir)
	if err != nil && !s.dev {
		return nil, err
	}
	s.cur = m
	s.stamp = stampOf(filepath.Join(distDir, buildfiles.Manifest))
	return s, nil
}

// Dev reports whether the source watches its manifest.
func (s *Source) Dev() bool { return s != nil && s.dev }

// Current returns the manifest to render with, or nil when none is loaded.
func (s *Source) Current() *Manifest {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Notify sends dev frames to w's sessions on build changes and starts the
// watcher on first call; prod sources ignore it.
func (s *Source) Notify(w *live.World) {
	if s == nil || !s.dev || w == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worlds = append(s.worlds, w)
	if !s.started {
		s.started = true
		s.stop = make(chan struct{})
		go s.watch(s.stop, 150*time.Millisecond)
	}
}

// Close stops the watcher.
func (s *Source) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
}

func (s *Source) watch(stop <-chan struct{}, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.poll()
		}
	}
}

func (s *Source) poll() {
	var frames []live.DevFrame

	msg := readDevError(filepath.Join(s.dir, buildfiles.DevError))
	stamp := stampOf(filepath.Join(s.dir, buildfiles.Manifest))

	s.mu.Lock()
	if msg != "" && msg != s.errMsg {
		frames = append(frames, live.DevFrame{What: "error", Message: msg})
	}
	cleared := s.errMsg != "" && msg == ""
	s.errMsg = msg
	if msg == "" && stamp != "" && stamp != s.stamp {
		if m, err := Load(s.dir); err == nil {
			s.stamp = stamp
			if f, ok := devChange(s.cur, m); ok {
				frames = append(frames, f)
			}
			s.cur = m
		}
		// A half-written tc.json fails to parse: retry on the next tick.
	}
	if cleared && len(frames) == 0 {
		// The fix restored the previous output: still drop the overlay.
		frames = append(frames, live.DevFrame{What: "reload"})
	}
	worlds := append([]*live.World(nil), s.worlds...)
	s.mu.Unlock()

	for _, f := range frames {
		for _, w := range worlds {
			w.Dev(f)
		}
	}
}

func devChange(old, cur *Manifest) (live.DevFrame, bool) {
	if old == nil {
		return live.DevFrame{What: "reload"}, true
	}
	for name, om := range old.Modules {
		nm, ok := cur.Modules[name]
		if !ok || om.File != nm.File || !sameList(om.Imports, nm.Imports) {
			return live.DevFrame{What: "reload"}, true
		}
	}
	for name := range cur.Modules {
		if _, ok := old.Modules[name]; !ok {
			return live.DevFrame{What: "reload"}, true
		}
	}
	css := map[string]string{}
	for name, om := range old.Modules {
		nm := cur.Modules[name]
		if len(om.CSS) != len(nm.CSS) {
			return live.DevFrame{What: "reload"}, true
		}
		for i := range om.CSS {
			if om.CSS[i] != nm.CSS[i] {
				css[om.CSS[i]] = nm.CSS[i]
			}
		}
	}
	if len(css) == 0 {
		return live.DevFrame{}, false
	}
	return live.DevFrame{What: "css", CSS: css}, true
}

func sameList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stampOf(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d/%d", fi.ModTime().UnixNano(), fi.Size())
}

func readDevError(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) != nil || e.Message == "" {
		return "build failed"
	}
	return e.Message
}
