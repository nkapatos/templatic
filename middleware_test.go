package templatic_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/internal/pagestate"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"
)

func renderString(t *testing.T, ctx context.Context, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestWithPageTokensSeedsAndBoots(t *testing.T) {
	w := live.NewWorld(live.WithKey([]byte("0123456789abcdef0123456789abcdef")))
	open := live.DeclareSessionCell("assistant.open", "")
	var page string
	h := templatic.WithPageTokens(w)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		templatic.SeedPage(r, open.Seed("open"))
		page = renderString(t, r.Context(), templatic.Boot(templatic.BootOptions{Live: "/live", Assets: "/a"}))
		if templatic.PageToken(r.Context()) == "" {
			t.Fatal("token must be memoized, not empty")
		}
		defer func() {
			if recover() == nil {
				t.Fatal("SeedPage after mint must panic")
			}
		}()
		templatic.SeedPage(r, open.Seed("closed"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(page, `"token":"`) {
		t.Fatalf("Boot must carry the ctx token: %s", page)
	}
	if templatic.PageToken(context.Background()) != "" {
		t.Fatal("outside the middleware there is no token")
	}
}

func TestBootWithoutLiveIsStaticUnderPageTokens(t *testing.T) {
	w := live.NewWorld(live.WithKey([]byte("0123456789abcdef0123456789abcdef")))
	var page string
	h := templatic.WithPageTokens(w)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		page = renderString(t, r.Context(), templatic.Boot(templatic.BootOptions{Assets: "/a"}))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if strings.Contains(page, `"token"`) {
		t.Fatalf("a page without Live must carry no token, so it opens no stream: %s", page)
	}
}

func TestWithAssetsBootPreloadsRenderedIslands(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tc.json"), []byte(`{"version":2,"modules":{"islands/x-card":{"file":"islands/x-card-1.js"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := templatic.ManifestFor("prod", dir)
	if err != nil {
		t.Fatal(err)
	}
	boot := templatic.Boot(templatic.BootOptions{Live: "/live", Assets: "/a"}) // one component, two requests
	var page string
	h := templatic.WithAssets(src)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			if r.URL.Query().Get("island") == "1" {
				if err := ui.Island("x-card", nil, nil).Render(ctx, w); err != nil {
					return err
				}
			}
			return boot.Render(ctx, w)
		})
		templ.Handler(body).ServeHTTP(rw, r)
		page = rw.(*httptest.ResponseRecorder).Body.String()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/?island=1", nil))
	if !strings.Contains(page, `<link rel="modulepreload" href="/a/islands/x-card-1.js">`) || !strings.Contains(page, `"manifest":`) {
		t.Fatalf("Boot must take the manifest from ctx and preload the rendered island: %s", page)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if strings.Contains(page, "modulepreload") {
		t.Fatalf("a preload leaked into the next request: %s", page)
	}
}

func TestFormForCarriesASubmissionIDPerRender(t *testing.T) {
	w := live.NewWorld(live.WithKey([]byte("0123456789abcdef0123456789abcdef")))
	cmd := live.NewCommand[struct {
		Title string `json:"title"`
	}]("note.add")
	var ids []string
	h := templatic.WithPageTokens(w)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		page := renderString(t, r.Context(), ui.FormFor(cmd, "/notes"))
		_, rest, ok := strings.Cut(page, `<input type="hidden" name="_tc_submit" value="`)
		id, _, _ := strings.Cut(rest, `"`)
		if !ok || id == "" {
			t.Fatalf("no submission id: %s", page)
		}
		ids = append(ids, id)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if ids[0] == ids[1] {
		t.Fatal("each render mints its own id")
	}
	if page := renderString(t, context.Background(), ui.FormFor(cmd, "/notes")); strings.Contains(page, "_tc_submit") {
		t.Fatalf("outside WithPageTokens there is no id: %s", page)
	}
}

func TestAppStack(t *testing.T) {
	var sawTokens, sawAssets bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps := pagestate.From(r.Context())
		sawTokens = ps != nil && ps.Tokens != nil
		sawAssets = ps != nil && ps.Manifests != nil
	})
	for _, tc := range []struct {
		name           string
		world          *live.World
		src            *templatic.ManifestSource
		tokens, assets bool
	}{
		{"full", live.NewWorld(), &templatic.ManifestSource{}, true, true},
		{"bare", nil, nil, false, false},
	} {
		templatic.App(tc.world, tc.src, inner).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		if sawTokens != tc.tokens || sawAssets != tc.assets {
			t.Fatalf("%s: tokens=%v assets=%v", tc.name, sawTokens, sawAssets)
		}
	}
}

func TestMiddlewareCompositionMatchesApp(t *testing.T) {
	var sawTokens, sawAssets bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps := pagestate.From(r.Context())
		sawTokens = ps != nil && ps.Tokens != nil
		sawAssets = ps != nil && ps.Manifests != nil
	})
	w, src := live.NewWorld(), &templatic.ManifestSource{}
	for name, h := range map[string]http.Handler{
		"composed": templatic.WithAssets(src)(templatic.WithPageTokens(w)(inner)),
		"App":      templatic.App(w, src, inner),
	} {
		sawTokens, sawAssets = false, false
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		if !sawTokens || !sawAssets {
			t.Fatalf("%s: tokens=%v assets=%v", name, sawTokens, sawAssets)
		}
	}
}

func TestHeadAndBootWriteEachLinkOnce(t *testing.T) {
	dir := t.TempDir()
	tc := `{"version":2,"base":"/a",
"modules":{
 "boot":{"file":"boot-1.js","imports":["chunks/s-1.js"]},
 "islands/x-listed":{"file":"islands/x-listed-1.js","css":["islands/x-listed-1.css"],"imports":["chunks/s-1.js"]},
 "islands/x-late":{"file":"islands/x-late-1.js","css":["islands/x-late-1.css"],"imports":["chunks/s-1.js","chunks/l-1.js"]}},
"integrity":{"boot-1.js":"sha384-boot","islands/x-listed-1.js":"sha384-listed","islands/x-late-1.js":"sha384-late","islands/x-late-1.css":"sha384-latecss","chunks/l-1.js":"sha384-l"}}`
	if err := os.WriteFile(filepath.Join(dir, "tc.json"), []byte(tc), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := templatic.ManifestFor("prod", dir)
	if err != nil {
		t.Fatal(err)
	}
	head := templatic.Head(templatic.HeadOptions{Assets: "/a", Preload: []string{"x-listed"}})
	boot := templatic.Boot(templatic.BootOptions{Live: "/live", Assets: "/a"})
	var page string
	h := templatic.WithAssets(src)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			if err := head.Render(ctx, w); err != nil {
				return err
			}
			for _, tag := range []string{"x-listed", "x-late"} {
				if err := ui.Island(tag, nil, nil).Render(ctx, w); err != nil {
					return err
				}
			}
			return boot.Render(ctx, w)
		})
		templ.Handler(body).ServeHTTP(rw, r)
		page = rw.(*httptest.ResponseRecorder).Body.String()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	for _, url := range []string{"boot-1.js", "chunks/s-1.js", "islands/x-listed-1.js", "islands/x-listed-1.css", "islands/x-late-1.js", "islands/x-late-1.css", "chunks/l-1.js"} {
		if n := strings.Count(page, `href="/a/`+url+`"`); n != 1 {
			t.Errorf("%s written %d times, want once: %s", url, n, page)
		}
	}
	for _, want := range []string{
		`<link rel="modulepreload" href="/a/boot-1.js" integrity="sha384-boot">`,           // Head
		`<link rel="stylesheet" href="/a/islands/x-listed-1.css">`,                         // Head, not repeated by Boot
		`<link rel="modulepreload" href="/a/islands/x-late-1.js" integrity="sha384-late">`, // Boot
		`<link rel="preload" as="style" href="/a/islands/x-late-1.css" integrity="sha384-latecss">`,
		`<link rel="modulepreload" href="/a/chunks/l-1.js" integrity="sha384-l">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %s in %s", want, page)
		}
	}
	if strings.Contains(page, `rel="preload" as="style" href="/a/islands/x-listed-1.css"`) {
		t.Error("Boot repeated a Head-listed island's css")
	}
	if strings.Index(page, "x-late-1.js") < strings.Index(page, "islands/x-listed-1.js") {
		t.Error("Head links must precede Boot's")
	}
}
