// Command production is a tiny live app wrapped the way it ships: gzip in
// front, cacheable-but-revalidated HTML, HTTP/2 without TLS for a proxy,
// timeouts that leave the live stream alone, and a graceful shutdown.
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic"
	"github.com/nkapatos/templatic/live"
	"github.com/nkapatos/templatic/ui"

	"examples/production/components"
)

// distDir is where `templatic generate` writes the bundle and tc.json.
const distDir = ".templatic/dist"

func main() {
	addr := ":8080"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}

	// Page tokens are HMAC-signed. Set TC_KEY (32+ random bytes) in
	// production; without it a per-process development key is used and
	// pages must reload after every restart.
	opts := []live.Option{live.WithRequestHeaders("X-Forwarded-For")}
	if key := os.Getenv("TC_KEY"); key != "" {
		opts = append(opts, live.WithKey([]byte(key)))
	}
	w := live.NewWorld(opts...)
	count := live.NewCell(w, "count", 0)

	// TC_MODE=dev follows `templatic dev` rebuilds and reloads open pages;
	// otherwise the manifest is read once.
	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), distDir)
	if err != nil {
		log.Fatalf("production: %v (run `go tool templatic generate` first)", err)
	}
	manifests.Notify(w)

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", page(func(*http.Request) templ.Component { return components.Home(count, nil) }))
	mux.Handle("POST /add", components.Add.Form(w,
		func(r *http.Request, _ *live.Session, in components.AddIn) (string, error) {
			count.Update(func(v int) int { return v + in.N })
			log.Printf("production: add %d from %s", in.N, clientAddr(r.Header, r.RemoteAddr))
			return "/", nil
		},
		func(rw http.ResponseWriter, r *http.Request, err error) {
			page(func(r *http.Request) templ.Component { return components.Home(count, ui.Failed(r, err)) },
				templ.WithStatus(http.StatusUnprocessableEntity)).ServeHTTP(rw, r)
		}))
	w.Mount(mux, "/live")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))
	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/", templatic.Assets(distDir)))

	// Shutdown waits for open requests, and a live stream never ends on its
	// own: cancelling the base context ends every stream when Shutdown starts.
	base, cancel := context.WithCancel(context.Background())
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	srv := &http.Server{
		Addr:              addr,
		Handler:           gzipHandler(templatic.App(w, manifests, mux)),
		Protocols:         &protocols,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: it would cut every live stream; the stream sets
		// its own deadline per frame.
		BaseContext: func(net.Listener) context.Context { return base },
	}
	srv.RegisterOnShutdown(cancel)

	stop, unnotify := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer unnotify()
	go func() {
		<-stop.Done()
		ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("production: shutdown: %v", err)
		}
	}()

	log.Printf("production: listening on %s", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-stop.Done()
}

// page renders a component per request as HTML that browsers may keep
// (bfcache) but must revalidate.
func page(c func(*http.Request) templ.Component, opts ...func(*templ.ComponentHandler)) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Cache-Control", "no-cache")
		templ.Handler(c(r), opts...).ServeHTTP(rw, r)
	})
}

// clientAddr is the address the proxy appended last to X-Forwarded-For,
// or the peer address without a proxy.
func clientAddr(h http.Header, remote string) string {
	xff := h.Values("X-Forwarded-For")
	if len(xff) == 0 {
		return remote
	}
	last := xff[len(xff)-1]
	return strings.TrimSpace(last[strings.LastIndex(last, ",")+1:])
}
