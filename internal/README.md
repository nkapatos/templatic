# internal

Packages that only templatic's own modules import. Their doc comments are for
people working on templatic; the public API is described in the
[root README](../README.md) and on
[pkg.go.dev](https://pkg.go.dev/github.com/nkapatos/templatic).

| Package | What it does | Used by |
| --- | --- | --- |
| [buildfiles](buildfiles) | Names of the files written into dist: tc.json, tc-error.json, .generations.json. | `generate`, `manifest`, `cli/internal/devcmd` |
| [clientruntime](clientruntime) | Embeds the browser runtime's TypeScript sources and its npm dependency ranges; also the TypeScript project. See its [README](clientruntime/README.md). | `generate`, `cli/internal/initcmd` |
| [fingerprint](fingerprint) | Short SHA-256 content hashes of asset files, with a cache. | root `templatic` |
| [form](form) | Decodes submitted forms into a command's input struct and reports field errors. | `live`, `ui` |
| [livehook](livehook) | Function variables that let `live/livetest` reach `live` internals. | `live`, `live/livetest` |
| [manifest](manifest) | Loads the asset manifest (tc.json); in dev it reloads on rebuilds. | root `templatic`, `pagestate` |
| [markup](markup) | Attribute escaping, name grammar and cell value rendering. | root `templatic`, `ui`, `generate` |
| [pagestate](pagestate) | Per-request page state: tokens, manifest source, rendered islands. | root `templatic`, `ui` |
| [testapp](testapp) | Copies the fixture app from `generate/testdata/app` for tests. | tests in `generate`, `cli`, root `templatic` and `plugins/*` |

The CLI's commands live in their own internal packages under
[cli/internal](../cli/internal), each exposing a `Command` for the `cli` package:

| Package | Command |
| --- | --- |
| [checktscmd](../cli/internal/checktscmd) | `templatic check-ts` |
| [cleancmd](../cli/internal/cleancmd) | `templatic clean` |
| [devcmd](../cli/internal/devcmd) | `templatic dev` |
| [generatecmd](../cli/internal/generatecmd) | `templatic generate`, plus the flags `dev` shares |
| [initcmd](../cli/internal/initcmd) | `templatic init`; its `scaffold/` directory holds the embedded templates |
