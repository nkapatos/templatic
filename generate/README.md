# generate

The templatic build step. `generate.Run` turns `.templ` files and the app's
client code into Go glue (`*_tc.go`), island TypeScript, a copy of the browser
runtime and an esbuild bundle with a `tc.json` manifest. The `templatic
generate` and `templatic dev` commands (see [`cli`](../cli)) call it.

The package documentation covers what `Run` does step by step, the options,
the island pragma and the plugin API:
<https://pkg.go.dev/github.com/nkapatos/templatic/generate>

Plugins that ship with templatic are in [`plugins/`](../plugins).

## Files

| File | Holds |
|---|---|
| `doc.go` | the package documentation |
| `generate.go` | `Options`, `Mode`, `Island`, `Summary` and `Run`; finding `.templ` files; removing orphaned generated files |
| `layout.go` | `GenDir` (`.templatic`), the marker file, path checks, dist generations, atomic writes |
| `render_file.go` | per-file work: parse, pragmas, script and style takeover, writing `_templ.go` and `_tc.go` |
| `pragma.go` | the `//templatic:island` pragma parser |
| `region_pragma.go` | the `//templatic:region` pragma and the region declaration it emits |
| `gen_go.go` | the `_tc.go` writer: island constructors, imports, unresolved qualifier check |
| `program.go` | type-checks the module with `go/packages` and collects calls into `live` |
| `cells.go` | checks island `bind=` topics against declared cells |
| `commands.go` | writes `.templatic/commands.ts` from `live.NewCommand` calls |
| `props.go` | converts a props struct to TypeScript interfaces, following `encoding/json/v2` rules |
| `ts.go` | writes the generated island modules and the first-time define module |
| `relimports.go` | rebases relative imports of an island's script and style |
| `runtime_copy.go` | copies the client runtime and writes `tsconfig.json` |
| `clientdeps.go` | checks installed `idiomorph` and `lit` versions before bundling |
| `bundle.go` | the esbuild build, content hashing, `tc.json`, pruning old files |
| `assetplugin.go` | built-in esbuild plugin for `?url`, `?raw` and `?inline` imports and the default asset loaders |
| `plugin.go` | `Plugin`, `Builder`, the hooks, `Entries`, `Manifest`, `Result` |

## Tests

```sh
go test ./generate
```

The tests run the real pass, including esbuild, so the package takes about 15
seconds. They work on copies of the fixture app in `testdata/app` (a small
module with `node_modules` stubs) and compare against files in
`testdata/golden`. The `mise run test` task runs the whole repo with `-race`.
