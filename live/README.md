# live

The server-side state of a templatic app. A `World` holds cells, regions,
streams, commands and sessions; browsers connect over Server-Sent Events and
send commands back as POST requests.

API documentation: https://pkg.go.dev/github.com/nkapatos/templatic/live

## Neighbours

- `ui` renders the markup that binds to this state.
- The root package mounts a world into an app and serves the client bundle.
- `live/livetest` runs commands against an in-memory session in tests.
- `internal/livehook` is the seam that lets `livetest` reach unexported
  session internals (see `testhook.go`).

## Files

| File | Holds |
| --- | --- |
| `world.go` | `World`, `NewWorld`, world options, `Mount`, `SSE`, `Commands`, topic checks, lifecycle callbacks |
| `cell.go` | `Cell`, `NewCell`, deprecated `SessionCell` |
| `family.go` | `CellFamily`, a keyed set of cells |
| `region.go` | `Region`, `World.Swap` |
| `collection.go` | `Collection`, `CollectionFamily`, the op history ring, subscriptions |
| `value.go` | `Value`, `SessionValue` (server-only session values) |
| `declare.go` | `Command`, `DecodeForm`, `Command.Form`, session cell, value and region declarations |
| `commands.go` | `CommandHandler` and the POST handler that dispatches commands |
| `handle.go` | `InputError`, `Invalid`, `StatusError`, `Validator`, typed `Handle` |
| `input.go` | `Args`, decoding of JSON and form input, `InputErrors`, `WriteInputError` |
| `at.go` | `At` and `Push`, the "command moved the session to a URL" receipt |
| `submit.go` | submission ids that make a repeated form POST answer the first outcome |
| `upload.go` | `SessionFor`, `WriteInvalid` for multipart uploads |
| `session.go` | `Session`, event queueing, attach, detach, resume, the send loop |
| `stream.go` | the SSE handler behind `World.SSE`, and the collections a connecting page shows |
| `identity.go` | page tokens: `WithKey`, `PageToken`, `TokenParam`, `SeedCell` |
| `request.go` | `RequestInfo` and the header and cookie allowlists |
| `strings.go` | `Strings`, a JSON string-or-array decoder for form fields |
| `dev.go` | `DevFrame` and `World.Dev`, the dev-loop notice |
| `wire.go` | wire event kinds (`KindCell`, `KindSwap`, ...) and payload shapes |
| `testhook.go` | registers the `internal/livehook` functions used by `livetest` |
| `doc.go` | the package documentation |

## Tests

```sh
go test -race ./live/...
```

`mise run test` runs the whole module the same way. Tests in this directory
are behavior tests against a world and, where needed, `httptest`; the
`session_*_test.go` files cover resume, handoff, overflow and retirement.
