# templatic design

What templatic is, the rules it keeps, how its parts fit together, and why
the main choices were made. How to use each package is in its documentation
on [pkg.go.dev](https://pkg.go.dev/github.com/nkapatos/templatic); open work
is in the [issues](https://github.com/nkapatos/templatic/issues).

## What it is

A Go library on [templ](https://templ.guide) for server-rendered pages that
stay live:

- **templ stays the template language.** `templatic generate` reads templ's
  AST and emits through templ's own generator; it does not fork templ or
  re-parse its output. An app does not also run `templ generate`.
- **State lives on the server.** Cells (single values), regions (server
  renders morphed in place) and streams (keyed lists) are pushed to open
  pages over Server-Sent Events. User intents come back as commands over
  POST, with typed, validated input.
- **Islands are opt-in client code** for the parts of a page that need it.
  Hooks attach client behavior to server-rendered markup without an island.
- **Go declarations are the contract.** The TypeScript types of island props
  and command input are generated from the Go types.

## Rules

1. **A library, not a server or a framework.** Everything templatic exposes
   is an `http.Handler`, a `func(http.Handler) http.Handler` middleware, a
   templ component or a function. The app owns `main`, the server, the
   router, the middleware order and its package layout.
2. **Standard `net/http`.** No web-framework or CSS-tool integration; other
   stacks are recipes in the docs. A dependency of the library needs a reason
   that holds for every app that imports it.
3. **HTTP policy is the app's.** Compression, page caching and the app's own
   static files are the app's. templatic sets headers only on what it serves:
   the bundle under `/_templatic/` and the live channel.
4. **The generator reads the app; it does not shape it.** It scans the
   packages under the root. There are no required directories, package names
   or registries, and every generated binding is a call to a public API the
   app could write by hand.
5. **Generated files are never edited, and the app's files are never
   overwritten.** `x_templ.go` and `x_tc.go` sit beside each `x.templ` and are
   committed, as with templ. Everything else templatic generates is in
   `.templatic/`, which is gitignored and safe to delete. An island's define
   module (`client/islands/{tag}.ts`) is created once and is the app's from
   then on.
6. **The client runtime is templatic's.** It ships inside the Go module and
   moves with the server's wire protocol. The app's own client code and
   libraries are independent of it.
7. **Every public package has a reason to be public:** an app imports it or
   extends templatic through it. Everything else is under `internal/`.

## Architecture

```
build    .templ + Go ──► templatic generate
                          ├─ templ generator ──► x_templ.go
                          ├─ pragmas ─────────► x_tc.go (island constructors, session regions)
                          ├─ go/types ────────► .templatic/commands.ts, .templatic/islands/*
                          ├─ runtime copy ────► .templatic/runtime
                          └─ esbuild + plugins ► .templatic/dist (hashed files, tc.json)

serve    templatic.App(world, manifests, mux)
           ├─ app pages: templ components with ui.*, templatic.Head and Boot
           ├─ World.Mount: GET {base}/stream (SSE), POST {base}/commands
           └─ templatic.Assets: .templatic/dist under /_templatic/

browser  runtime ◄── SSE: cell, swap, append, ready, resync, dev
                 ──► POST: commands
```

`generate` stops at the first error and leaves what it already wrote in
place. The bundle has one entry for the runtime's boot module, one per
island, `client/app.ts` and each `client/modules/*.ts`; plugins can add
entries and hook every stage. The steps are in the
[generate](https://pkg.go.dev/github.com/nkapatos/templatic/generate) docs,
and the mounting order is in the
[templatic](https://pkg.go.dev/github.com/nkapatos/templatic) docs.

## Wire protocol

Defined in `live/wire.go`, `live/commands.go` and
`internal/clientruntime/ts`. It is not a stable API before v1.

**Down (SSE).** One event type, `tc`, whose data has a `kind`:

| Kind | Carries |
|---|---|
| `cell` | a cell's new value |
| `swap` | a region's new markup, morphed in place with idiomorph |
| `append` | keyed stream items |
| `ready` | the initial state has been sent |
| `resync` | the session cannot be resumed; the page reloads |
| `dev` | dev loop only: reload, CSS swap, build error |

**Up (POST).** A JSON body `{sid, action, args}`, or `form` (a URL-encoded
form body) instead of `args`, plus `submit` (the form's submission id).

| Answer | When |
|---|---|
| 204 | done, no result |
| 200 `{"result": …}` or `{"url": …}` | a result, or a URL to push (`live.Push`) |
| 400 | a bad payload, or refused input with its field errors |
| 404 | unknown command |
| 409 | unknown session; the client reloads |
| 413 | payload over 1 MiB |
| 4xx/5xx from `live.StatusError` | the handler's own status |
| 500 | any other error; the message is not sent |

Contracts apps rely on:

- **Identity is server-minted.** A page carries a page token signed with the
  world's key (`live.WithKey`; set it in production so tokens survive
  restarts). A stream without a valid token gets 403.
- **Sessions survive a dropped connection** for the resume grace (30 s by
  default, `live.WithResumeGrace`). On reattach the server resends cells,
  the latest missed swap per topic, and stream items by state. A client that
  is too late, or that fell too far behind, gets `resync`.
- **Cells keep the latest value per topic** within a burst: publish whole
  values, never deltas.
- **Reach:** session cells and session regions reach one session; world
  cells, `World.Redraw` and `World.Append` reach every session. A page
  without the target in its markup ignores the event.
- **Islands inside a swapped region remount** as units, so the morph never
  mixes server markup into an island's live DOM.
- **A form submission runs at most once per session.** `ui.FormFor` renders
  a signed submission id; a repeat (the runtime resubmits natively after a
  5xx or network error, or a user double-submits without JS) waits for the
  first run and gets its outcome. A success answers `X-TC-Submit` with a
  fresh id for the next submission.

## Decisions

- **The runtime is embedded and bundled by the app** (the Phoenix model).
  `generate` copies it from the Go module into `.templatic/runtime`, and
  esbuild bundles it with the islands and the app's client code: one bundle,
  one runtime instance, one `lit`, always matching the server's version.
  A prebuilt runtime served separately was rejected: templatic would have to
  carry idiomorph itself and split the runtime from the islands.
- **`idiomorph` and `lit` are the app's npm dependencies.** The runtime
  declares them as peer ranges (`idiomorph ~0.8.0`, `lit ^3.3.3`); `init`
  writes the tested versions, and `generate` fails with the `npm install`
  command when an installed version is out of range (`lit` only when the app
  has a lit island).
- **Standard middleware shape.** `WithAssets(src)` and `WithPageTokens(w)`
  return `func(http.Handler) http.Handler`, which every net/http router
  accepts as is; `App` stacks the two.
- **No page or HTTP helpers that templ or the app already have.** Pages
  render with `templ.Handler` (buffered, so a render error answers 500). There
  is no compression middleware: compression is app or proxy work, and the
  live stream flushes every frame and sends `X-Accel-Buffering: no`, so a
  flush-aware compressor delivers frames at once. There is no page-cache
  header and no serving of the app's static files.
- **Islands are marked with a pragma**, `//templatic:island tag=… props=…`,
  directly above the component. It carries what a naming convention cannot:
  a stable element name, a props struct, the variant and the binds.
  Generating a lit island's `render()` from its templ markup is
  [#1](https://github.com/nkapatos/templatic/issues/1).
- **One manifest layout and one link writer.** `tc.json` (version 2) lists
  every entry with its file, CSS and imports. `Head` and `Boot` write
  links through one function that de-duplicates per request: `Head` writes
  what the page lists up front, and `Boot` adds the islands the page
  actually rendered.
- **The generator reads Go through the type checker** (`go/packages`,
  `go/types`) after templ generation: real types across packages, aliases,
  embedded fields, constants and build tags, instead of resolving syntax by
  hand.
- **`encoding/json/v2` everywhere.** Nil slices encode as `[]`, so the
  generated TypeScript types are true; input names match exactly, and
  duplicate names, trailing data and invalid UTF-8 are errors. Compared or
  signed bytes (the manifest, tokens, submission ids) are deterministic, and
  JSON inside `<script>` is HTML-escaped. A type v2 cannot encode in props
  or command input fails `generate`, naming the field.
- **One CLI entry point.** The `cli` package's `Main` runs every subcommand;
  an app that wants plugins builds its own binary with
  `cli.Main(args, stdout, stderr, cli.With(plugins...))`. Plugins come only
  through `With`; there are no plugin flags. The CLI uses `urfave/cli/v3`
  (no dependencies of its own) behind `cli`'s API, and `dev` watches with
  `fsnotify`.
- **The client SDK's surface is locked** by an API Extractor report
  (`internal/clientruntime/etc/clientruntime.api.md`) that `mise run check`
  compares, so an export change is deliberate and reviewed. Runtime tests
  run in Vitest with happy-dom and the real idiomorph.
- **Versions are semver tags**, released from `main`; see
  [AGENTS.md](AGENTS.md#releases).
