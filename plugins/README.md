# plugins

First-party plugins for [`generate`](../generate). This directory is not a Go
package; each plugin is its own package below it.

| Package | What it does | Hook |
|---|---|---|
| [`budget`](budget) | prints each JS entry's gzip size and fails the build when an entry is over its limit (`budget.Limit`) | `OnDone` |
| [`env`](env) | exposes environment variables that start with a prefix to client code as `process.env.NAME` defines | `OnConfig` |
| [`sri`](sri) | records a sha384 Subresource Integrity hash for every `.js` and `.css` output in `tc.json` | `OnManifest` |

API docs: [budget](https://pkg.go.dev/github.com/nkapatos/templatic/plugins/budget),
[env](https://pkg.go.dev/github.com/nkapatos/templatic/plugins/env),
[sri](https://pkg.go.dev/github.com/nkapatos/templatic/plugins/sri).

## Using a plugin

The stock `templatic` binary has no plugins. Give the app its own build
program (see [`cli`](../cli)):

```go
os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr, cli.With(sri.New())))
```

Or pass them to `generate.Run` in `generate.Options.Plugins`.

## Writing a plugin

A plugin implements `generate.Plugin`: a `Name() string` and a
`Setup(*generate.Builder) error`. In `Setup`, register hooks on the builder
(`OnConfig`, `OnScan`, `OnEntries`, `ESBuild`, `OnBuildOptions`, `OnManifest`,
`OnDone`, `Watch`). The three plugins here are the shortest examples: each is
one file and one hook. Configure a plugin through `New(...)` and options, as
`budget` does, not through globals.

The hooks, when they run and the types they receive are in the
[`generate.Plugin` documentation](https://pkg.go.dev/github.com/nkapatos/templatic/generate#Plugin).

## Tests

```sh
go test ./plugins/...
```
