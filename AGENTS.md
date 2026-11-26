# templatic: notes for contributors and coding agents

templatic is a Go library on templ: server-rendered pages kept live over SSE,
typed commands, islands, and a `generate` build step. Start with
[README.md](README.md); each package directory has its own README (file map,
tests) and its package doc (`go doc ./<pkg>`).

## Commands

```sh
mise install        # pinned go, node and tools (mise.toml)
mise run check      # every check; must pass on a clean tree before a commit
mise run test       # go test -race -shuffle=on ./...
mise run fmt-fix    # gofmt -w .
```

`check` runs: build, fmt, tidy, vet, lint (staticcheck, exhaustive),
lint-actions (actionlint), vuln (govulncheck, needs network), the Go tests,
and for the TypeScript runtime in `internal/clientruntime`: typecheck, Vitest
and the API Extractor report. It ends by asserting the tree is clean, so
commit first, then run it. CI runs the same `mise run check`.

## Conventions

- **Library, not framework.** Apps own `main`, the server, the router and the
  middleware order; templatic hands them `net/http` handlers and middleware.
  Runtime code uses the standard library (`net/http`, `encoding/json/v2`); a
  new dependency needs a reason that holds for every app that imports it.
- **Public packages have a reason to be public** (an app imports them or
  extends through them); everything else goes under `internal/`. Moving or
  renaming packages is a design decision: propose it, do not just do it.
- **Dev tools are pinned in mise.toml**, not in go.mod, so they stay out of
  apps' module graphs. (staticcheck, exhaustive and templ are in go.mod's
  tool block.)
- **Tests** check behavior through the public API, with the standard
  `testing` package (no assertion libraries). Shared fixture code for tests
  lives in `internal/testapp`.
- **Comments:** an exported identifier gets one line stating its contract;
  unexported code gets a comment only for a non-obvious why. Package docs
  (doc.go) carry the overview pkg.go.dev shows. Fix a wrong comment when you
  see it.
- **Imports:** standard library in one group, then other modules.
- **TypeScript runtime API:** an intended change to `internal/clientruntime`'s
  exports updates the report with `npm run api:update` and commits it with
  the change.
- **Generated files:** the test fixtures' `*_templ.go`, `*_tc.go` and
  `.templatic/` are generated during tests and never committed.

## Agent skills

Go skills for coding agents are vendored in [.claude/skills](.claude/skills)
(from samber/cc-skills-golang, MIT). Where a skill and this file disagree,
this file wins: for example no testify, no DI frameworks, no samber libraries.
