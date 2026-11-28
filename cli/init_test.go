package cli

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nkapatos/templatic/internal/clientruntime"
)

func TestInitRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "x"), nil, 0o644)
	var out, errb bytes.Buffer
	if c := Main([]string{"init", "-replace", "..", dir}, &out, &errb); c != 1 || !strings.Contains(errb.String(), "not empty") {
		t.Fatalf("code %d: %s", c, errb.String())
	}
	if c := Main([]string{"init", "-replace", "..", "-force", dir}, &out, &errb); c != 0 {
		t.Fatalf("force: %d %s", c, errb.String())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "x")); len(b) != 0 {
		t.Fatal("-force overwrote a file")
	}
}

func TestInitDevBuildNeedsVersionOrReplace(t *testing.T) {
	var out, errb bytes.Buffer
	if c := Main([]string{"init", filepath.Join(t.TempDir(), "app")}, &out, &errb); c != 1 || !strings.Contains(errb.String(), "-replace") {
		t.Fatalf("code %d: %s", c, errb.String())
	}
}

func TestInitSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a scaffolded app")
	}
	checkout, _ := filepath.Abs("..")
	nodeModules := filepath.Join(checkout, "internal", "clientruntime", "node_modules")
	if _, err := os.Stat(filepath.Join(nodeModules, "idiomorph")); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("internal/clientruntime/node_modules not installed: CI must run the init smoke test")
		}
		t.Skip("internal/clientruntime/node_modules not installed")
	}
	dir := filepath.Join(t.TempDir(), "smokeapp")
	var out, errb bytes.Buffer
	if c := Main([]string{"init", "-module", "example.com/smokeapp", "-replace", checkout, dir}, &out, &errb); c != 0 {
		t.Fatalf("init: %d %s", c, errb.String())
	}
	if gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(gi) != "node_modules/\n.templatic/\n" {
		t.Fatalf(".gitignore = %q", gi)
	}
	if err := os.Symlink(nodeModules, filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	goCmd := func(args ...string) {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Env = dir, env
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, b)
		}
	}
	goCmd("mod", "tidy")
	if c := Main([]string{"generate", "-root", dir}, &out, &errb); c != 0 {
		t.Fatalf("generate: %d %s", c, errb.String())
	}
	goCmd("vet", "./...")
	if c := Main([]string{"check-ts", "-root", dir}, &out, &errb); c != 0 {
		t.Fatalf("check-ts: %d %s %s", c, out.String(), errb.String())
	}
	bin := filepath.Join(t.TempDir(), "smokeapp")
	goCmd("build", "-o", bin, ".")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	srv := exec.Command(bin)
	srv.Dir, srv.Env = dir, append(env, "ADDR="+addr, "TC_KEY=smoke-test-key-0123456789abcdef0123")
	var logs bytes.Buffer
	var logMu sync.Mutex
	lw := &lockedWriter{mu: &logMu, w: &logs}
	srv.Stdout, srv.Stderr = lw, lw
	logText := func() string { logMu.Lock(); defer logMu.Unlock(); return logs.String() }
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Process.Kill(); _ = srv.Wait() })
	base := "http://" + addr

	var page string
	for deadline := time.Now().Add(10 * time.Second); ; {
		if res, err := http.Get(base + "/"); err == nil {
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatalf("GET / = %d\n%s", res.StatusCode, b)
			}
			page = string(b)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never came up\n%s", logText())
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, want := range []string{`data-tc-form="count.add"`, `data-tc-hook="autofocus"`, `rel="stylesheet" href="/_templatic/app-`, `data-tc-cell="count"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("page lacks %s:\n%s", want, page)
		}
	}
	if res, err := http.Get(base + "/static/favicon.svg"); err != nil || res.StatusCode != 200 {
		t.Fatalf("app static file not served: %v %v", res, err)
	}
	if res, err := http.Get(base + "/_templatic/favicon.svg"); err != nil || res.StatusCode != 404 {
		t.Fatalf("templatic must not serve the app's files: %v %v", res, err)
	}

	tok := regexp.MustCompile(`"token":"([^"]+)"`).FindStringSubmatch(page)
	if tok == nil {
		t.Fatal("page carries no token")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(strings.SplitN(tok[1], ".", 2)[0])
	var claims struct {
		SID string `json:"s"`
	}
	_ = json.Unmarshal(raw, &claims)
	stream, err := http.Get(base + "/live/stream?c=count&t=" + url.QueryEscape(tok[1])) // the page shows the count cell
	if err != nil || stream.StatusCode != 200 {
		t.Fatalf("stream: %v %v", stream, err)
	}
	defer stream.Body.Close()
	frames := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stream.Body)
		for sc.Scan() {
			if line, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				frames <- line
			}
		}
		close(frames)
	}()
	waitFrame := func(want string) {
		t.Helper()
		timeout := time.After(5 * time.Second)
		for {
			select {
			case f, ok := <-frames:
				if !ok {
					t.Fatalf("stream closed waiting for %s", want)
				}
				if strings.Contains(f, want) {
					return
				}
			case <-timeout:
				t.Fatalf("no frame %s", want)
			}
		}
	}
	waitFrame(`"kind":"ready"`)
	command := func(payload map[string]any) *http.Response {
		payload["sid"], payload["action"] = claims.SID, "count.add"
		body, _ := json.Marshal(payload)
		res, err := http.Post(base+"/live/commands", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	if res := command(map[string]any{"args": map[string]int{"n": 1}}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("typed count.add = %d", res.StatusCode)
	}
	waitFrame(`{"kind":"cell","topic":"count","v":1}`)
	if res := command(map[string]any{"form": "n=1"}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("form count.add = %d", res.StatusCode)
	}
	waitFrame(`{"kind":"cell","topic":"count","v":2}`)
	if res := command(map[string]any{"form": "n=500"}); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid count.add = %d, want 400", res.StatusCode)
	}

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noRedirect.PostForm(base+"/add", url.Values{"n": {"3"}})
	if err != nil || res.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /add: %v %v", res, err)
	}
	waitFrame(`{"kind":"cell","topic":"count","v":5}`)
	res, err = noRedirect.PostForm(base+"/add", url.Values{"n": {"500"}})
	if err != nil || res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid POST /add: %v %v", res, err)
	}
	rerendered, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(rerendered), `value="500"`) || !strings.Contains(string(rerendered), "pick a step between -100 and 100</span>") {
		t.Fatalf("invalid POST /add must re-render the form with the value and the error:\n%s", rerendered)
	}
	if strings.Contains(logText(), "DEVELOPMENT key") {
		t.Fatal("TC_KEY was not used")
	}
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

func TestInitPinsClientDeps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	var out, errb bytes.Buffer
	if c := Main([]string{"init", "-replace", "..", dir}, &out, &errb); c != 0 {
		t.Fatalf("init: %d %s", c, errb.String())
	}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		t.Fatal(err)
	}
	want := clientruntime.TestedVersions()
	if len(pkg.Dependencies) != len(want) {
		t.Fatalf("dependencies %v, want %v", pkg.Dependencies, want)
	}
	for name, v := range want {
		if pkg.Dependencies[name] != v {
			t.Fatalf("dependencies %v, want %v", pkg.Dependencies, want)
		}
	}
}
