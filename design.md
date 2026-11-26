# templatic design

What templatic is, the rules it keeps, how its parts fit together, and why
it is shaped this way. How to use each package is in its package
documentation.

## What it is

A Go library on [templ](https://templ.guide) for server-rendered pages that
stay live.

- **templ stays the template language.** templatic reads templ's syntax tree
  and emits through templ's own generator. It does not fork templ or re-parse
  its output, and an app does not run templ separately.
- **State lives on the server.** Values, server-rendered fragments and keyed
  lists are pushed to every open page that shows them. User intents come
  back as commands with typed, validated input.
- **Client code is opt-in.** Islands are client components for the parts of
  a page that need them; hooks attach client behavior to server-rendered
  markup. Everything else is plain HTML from the server.
- **Go declarations are the contract.** The client-side types of island
  props and command input are generated from the Go types, so the two sides
  cannot disagree silently.

## Rules

1. **A library, not a server or a framework.** Everything templatic exposes
   is an `http.Handler`, a standard `func(http.Handler) http.Handler`
   middleware, a templ component or a function. The app owns `main`, the
   server, the router, the middleware order and its package layout.
2. **Standard `net/http`.** No web-framework or CSS-tool integration; other
   stacks are recipes. A dependency of the library needs a reason that holds
   for every app that imports it.
3. **HTTP policy is the app's.** Compression, page caching and the app's own
   static files belong to the app or its proxy. templatic sets headers only
   on what it serves itself: the client bundle and the live channel.
4. **The generator reads the app; it does not shape it.** No required
   directories, package names or registries. Every binding it generates is a
   call to a public API the app could write by hand.
5. **Generated files are never edited, and the app's files are never
   overwritten.** Generated Go sits beside its `.templ` file and is committed,
   as with templ. Everything else templatic generates lives in one directory
   it owns, which is not committed and is safe to delete. A starter file
   templatic creates for the app is the app's from then on.
6. **The client runtime is templatic's.** It ships inside the Go module and
   moves together with the server side of the wire protocol. The app's own
   client code and libraries are independent of it.
7. **Every public package has a reason to be public:** an app imports it or
   extends templatic through it. Everything else is internal.

## Architecture

```
build    .templ + Go ──► templatic generate
                          ├─ templ's generator ─► Go for each .templ
                          ├─ declarations ──────► Go glue (islands, regions)
                          ├─ Go types ──────────► TypeScript types (props, commands)
                          └─ bundler + plugins ─► client bundle + manifest

serve    app's mux, wrapped in templatic's middleware
           ├─ the app's pages, rendered with templ
           ├─ the live channel: an event stream down, commands up
           └─ the client bundle

browser  templatic runtime ◄── events: values, fragments, list items
                           ──► commands
```

The build is one step the app runs before it compiles; plugins hook into
it. At run time the app mounts templatic's handlers on its own mux; nothing
runs that the app did not mount.

## The live channel

Server-Sent Events carry state down; commands go up as POST requests. The
contracts apps rely on:

- **Identity is server-minted.** A page carries a signed token that ties it
  to its session; the client never chooses its own identity.
- **Reconnects restore state, not history.** A session that drops its
  connection keeps its place for a grace period; on reconnect it gets the
  current values and fragments, not a replay of every event. A client that
  cannot be caught up reloads.
- **Values are latest-wins.** A burst of changes to one value sends the last
  one. Values are published whole; incremental changes to a dataset are a
  keyed list, not a value.
- **Reach is explicit.** Session state reaches one session; world state
  reaches every session. A page without the target in its markup ignores
  the update.
- **Islands are units.** A server fragment that contains an island never
  morphs into the island's live DOM; the island remounts.
- **Forms degrade, and run once.** A command can also be served as a plain
  form post, so a page without JavaScript still works. A form submission runs at
  most once per session: a resubmission, by the runtime after a failure or
  by a user without JavaScript, gets the first run's outcome.

## Decisions

- **The runtime is embedded and bundled by the app**, as in Phoenix. The app
  bundles templatic's runtime with its islands and its own client code from
  its one `package.json`: one bundle, one runtime instance, one copy of each
  shared library, always matching the server's version. A prebuilt runtime
  served separately would make templatic carry its client libraries itself
  and split the runtime from the islands.
- **The runtime's client libraries are the app's npm dependencies.**
  templatic declares the range it supports, `init` writes the version it was
  tested with, and the build stops with the install command when an
  installed version is out of range.
- **No helpers that templ or the app already have.** Pages render with
  templ's own handler. There is no compression or cache-header middleware:
  that is the app's HTTP policy (rule 3). The live stream flushes every batch
  of frames as it writes it and opts out of proxy buffering, so the app's own
  compression does not hold it back.
- **Islands are marked with a pragma** directly above the component. It
  carries what a naming convention cannot: a stable element name independent
  of the Go name, a props struct, the island's variant and its bindings to
  server state.
- **The generator reads Go through the type checker**, after templ's
  generation: real types across packages, aliases, embedded fields,
  constants and build tags, instead of resolving syntax by hand.
- **JSON follows `encoding/json/v2`** on both sides of the wire. Empty
  collections are empty, not null, so the generated types are true; input
  is strict. A Go type that cannot cross the wire fails the build rather
  than the request.
- **One CLI entry point.** The `cli` package runs every subcommand, and an
  app that wants plugins builds its own binary around it with its plugins.
  Plugins come in only that way, never through flags. The CLI's argument
  library stays behind `cli`'s API, so it can change without an API change.
