# 12 · production

One shared count, wrapped the way it would ship: gzip in front of
`templatic.App`, revalidated HTML, HTTP/2 for a proxy, timeouts that leave
the live stream open, a graceful shutdown, a sample proxy config and a
container image.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

Open it in two windows and add in one.

## The dev loop

`npm run dev` (or `mise run dev`) runs `templatic dev` with `TC_MODE=dev`:

- it builds the bundle unminified with linked sourcemaps, then watches the
  directory;
- a change under `client/` rebuilds the bundle, and open pages pick up the
  new CSS or reload, without a restart;
- a `.templ` change regenerates its Go, and any `.go` change restarts the
  `-exec` command (`go run .`); open pages reload once it is back;
- with `TC_MODE=dev`, `templatic.ManifestFor` follows each rebuild and
  `manifests.Notify(w)` tells open pages over the live channel.

Ctrl-C stops it and the app (signalling only the `go tool` process does
not: [#52](https://github.com/nkapatos/templatic/issues/52)). To ship, run `go tool templatic generate`
(minified) and `go build`; without `TC_MODE` the manifest is read once.

## What it shows

- **Compression is the app's** ([design.md](../../design.md#rules),
  rule 3). `compress.go` is one self-contained, standard-library file
  that works in front of any `net/http` handler; copy it.
  `gzipHandler(templatic.App(w, manifests, mux))` in `main.go` puts it
  outermost. It:
  - sets `Vary: Accept-Encoding` on every response;
  - decides at the first write by `Content-Type`: text, JSON, JavaScript
    and SVG are compressed once they reach 512 bytes; images, fonts and
    anything already `Content-Encoding`d pass through;
  - skips `HEAD` and every status other than 200 (204, 304, errors);
  - drops `Content-Length` and `Accept-Ranges` and weakens a strong `ETag`
    when it compresses;
  - **never compresses `text/event-stream`.** The live stream flushes each
    batch of frames as it writes it; gzip keeps bytes until its window
    fills, so frames and heartbeats would wait in the compressor;
  - implements `Flush`, which decides at once so a streamed response is
    never held back, and `Unwrap`, so `http.ResponseController` reaches
    the connection for the stream's flushes and write deadlines.

  Compress in one place: here or in the proxy, not both.
- **HTML is `Cache-Control: no-cache`.** `page` in `main.go` sets it on
  the page and on the 422 re-render: browsers revalidate on every load
  and keep the page in the back/forward cache, which `no-store` would
  turn off. `templatic.Assets` sets its own policy (hashed files
  immutable, `boot.js` and `tc.json` `no-cache`); the stream is
  `no-store`.
- **`TC_KEY` signs page tokens.** Set the same 32+ random bytes on every
  replica and across deploys (`openssl rand -base64 32`), so a token
  minted by one process verifies on another. `live.WithKey` panics on
  a key shorter than 32 bytes. Without
  it the process logs that it uses a development key.
- **HTTP/2, so tabs do not run out of connections.** Over HTTP/1.1 every
  open tab's event stream holds one of the ~6 connections a browser
  opens per origin; a few tabs in, page loads and commands queue behind
  them. HTTP/2 carries every stream on one connection. Browsers speak it
  only over TLS: terminate TLS in the proxy (below), or serve TLS from Go
  with `srv.ListenAndServeTLS`. `http.Server.Protocols` in `main.go`
  accepts HTTP/1.1 and unencrypted HTTP/2 (h2c) on the same port, for a
  proxy that speaks h2c to the app; `curl --http2-prior-knowledge` talks
  to it directly.
- **Timeouts leave the stream open.** `ReadHeaderTimeout` and
  `IdleTimeout` are set; `WriteTimeout` is not: it bounds the whole
  response and would cut every stream. The stream sets its own deadline
  per frame (10s) and sends a heartbeat every 15s
  (`live.WithHeartbeat`), so a proxy's read timeout must be above 15s.
- **The proxy.** `Caddyfile` is a sample, not run here: TLS and HTTP/2 to
  browsers, h2c to the app, `read_timeout 60s`. Caddy flushes event
  streams as they arrive; nginx needs no `proxy_buffering off` for the
  stream because it sends `X-Accel-Buffering: no`, and its default
  `proxy_read_timeout` (60s) is already above the heartbeat.
- **The client's address.** `live.WithRequestHeaders("X-Forwarded-For")`
  allowlists the header, so command handlers can read it from
  `s.CommandRequest().Header`; `clientAddr` takes the last entry, the one
  your proxy added. Trust it only behind that proxy. The no-JS form
  handler reads `r.Header` directly.
- **Graceful shutdown.** On SIGTERM (a container stop) or Ctrl-C,
  `srv.Shutdown` waits for open requests, and a live stream never ends on
  its own: `BaseContext` and `RegisterOnShutdown(cancel)` end every stream
  when shutdown starts, so the process exits at once. Open pages
  reconnect to the next process and reload against its state.
- **Sticky routing for replicas.** Sessions, the cells' values and the
  run-once record of form submissions live in the process. A page's
  stream and its commands must reach the replica that rendered it, so the
  proxy pins a browser to one (`lb_policy cookie`). Each replica has its
  own world: state every viewer shares must come from a store all
  replicas read, and each replica sets its cells from it.
- **The container.** `Dockerfile`, multi-stage: `npm ci`, then
  `go tool templatic generate` and `go build`, then a distroless image
  with the binary, `.templatic/dist` and `public/`. It is written for an
  app that requires a templatic release: in this checkout `go.mod`
  replaces templatic with `../..`, outside the build context.
- **No single binary yet.** The bundle and its manifest cannot be served
  or read from an `embed.FS`, so they ship beside the binary
  ([#47](https://github.com/nkapatos/templatic/issues/47)).

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the command, the mux, `page`, the server, its timeouts and shutdown |
| `compress.go` | the gzip middleware |
| `components/count.go` | the `Add` command and `AddIn` |
| `components/home.templ` | the page: the count and the form |
| `components/layout.templ` | the document, `Head`, `Boot` |
| `client/app.ts`, `client/app.css` | the bundle's sources |
| `public/favicon.svg` | a file the app serves itself |
| `Caddyfile` | a sample TLS proxy in front of two sticky replicas |
| `Dockerfile`, `.dockerignore` | the container image |

`*_templ.go` files are generated by `templatic generate` and committed, as
with templ. `.templatic/` is generated and ignored.
