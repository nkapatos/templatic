# cli

The `templatic` command line as a Go package. `cmd/templatic` is a one-line
`main` around `cli.Main`; an app that wants plugins writes its own `main` the
same way and passes `cli.With(...)`.

Commands, flags, exit codes and the app build program are in the package
documentation: <https://pkg.go.dev/github.com/nkapatos/templatic/cli>

## Layout

| Path | Holds |
|---|---|
| `cli.go` | `Main`, `With`, `Options`; the root command and exit codes |
| `internal/generatecmd` | `templatic generate`, the flags it shares with `dev`, and how flags and plugins become `generate.Options` |
| `internal/devcmd` | `templatic dev`: file watching, rebuilds, restarting the `--exec` app |
| `internal/initcmd` | `templatic init`; the templates it writes are in `internal/initcmd/scaffold` (`*.tmpl`) |
| `internal/cleancmd` | `templatic clean` |
| `internal/checktscmd` | `templatic check-ts` |
| `testdata/app` | a small app the CLI tests build |

The build itself is in [`generate`](../generate); this package only parses
arguments and calls it. Commands are built with
[urfave/cli v3](https://github.com/urfave/cli).

## Tests

```sh
go test ./cli/...
```

`cli_test.go` covers help output, exit codes and flag spellings.
`clean_test.go` and `init_test.go` run those commands. `TestInitSmoke`
scaffolds an app, runs `go mod tidy` (module proxy access needed), generates
and builds it; it is skipped under `-short` or when
`internal/clientruntime/node_modules` is not installed (`mise run npm-install`).
The `internal/*cmd` packages have their own tests.
