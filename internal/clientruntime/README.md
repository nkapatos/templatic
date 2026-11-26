# clientruntime

The TypeScript source of templatic's browser runtime: live-channel handling,
commands, forms, islands, hooks and boot. It is a private npm project
(`@templatic/clientruntime`) inside the Go module. [clientruntime.go](clientruntime.go)
embeds `ts/*.ts` and `package.json`. `templatic generate` copies the sources into an
app's `.templatic/runtime`, and the app's bundle includes them. The Go API is on
[pkg.go.dev](https://pkg.go.dev/github.com/nkapatos/templatic/internal/clientruntime).

## ts/ modules

| File | Holds |
| --- | --- |
| `boot.ts` | Entry point: reads config, loads the manifest, wires the other modules. |
| `tc.ts` | Shared state: config, manifest types, boot phases, `tcState`, ready state, event bus, cells. |
| `connect.ts` | The live-channel EventSource with manual reconnect. |
| `live.ts` | Applies wire events: cell, swap and append updates, morphing, cell attributes. |
| `commands.ts` | Sends commands and uploads; `CommandError`; history replay receipts. |
| `actions.ts` | Click delegation for `data-tc-action` triggers. |
| `forms.ts` | Form submission and error marking. |
| `history.ts` | The popstate handler that replays pushed entries. |
| `hooks.ts` | `defineHook` and the hook lifecycle. |
| `islands.ts` | Loads island chunks and styles from the manifest. |
| `modules.ts` | Loads the `app` and `modules/<name>` entries. |
| `context.ts` | `IslandContext`: the scoped context islands and hooks receive. |
| `props.ts` | Reads an island's JSON props payload. |
| `dev.ts` | Dev frames: reload, CSS swap, build-error overlay. |
| `index.ts` | The public API (`cell`, `on`, `defineHook`, ...) that apps import. |
| `assets.d.ts`, `css.d.ts` | Type declarations for asset and CSS imports. |

## Develop

Run from this directory (needs the node version pinned in the root `mise.toml`):

    npm install
    npm run typecheck   # tsc over ts/ and test/
    npm test            # Vitest with happy-dom

Tests are `test/<module>.test.ts`; shared fakes (config, EventSource, fetch,
animation frames, morph recording) and the per-test cleanup are in `test/support/`.

The public API is checked with API Extractor against
[etc/clientruntime.api.md](etc/clientruntime.api.md):

    npm run api          # fails if the exported API differs from the report
    npm run api:update   # rewrites the report; commit it with the intended change

## mise tasks

Run from the repo root. Each depends on `npm-install`, which runs `npm install` once
if `node_modules` is missing.

| Task | Runs |
| --- | --- |
| `mise run typecheck` | `npm run typecheck` |
| `mise run test-ts` | `npm test` |
| `mise run api` | `npm run api` |

`mise run check` runs all three, along with the Go checks.
