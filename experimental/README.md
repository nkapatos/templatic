# experimental

Packages that may change or be removed without notice. Nothing else in
templatic depends on them.

| Package | What it is |
|---|---|
| [`pages`](pages) | request side of a file-based pages router: `Page` (a templ page as an `http.Handler`), `Ctx`, `Route`, `WithLayout` |
| [`routes`](routes) | build side: `Scan` a directory of `.templ` pages into a route table and `Write` it as `routes_gen.go` |

`routes` generates a `Routes` function returning `[]pages.Route`, and a
`Register` function for `net/http.ServeMux`. The file naming rules (path from
file path, trailing `_` for parameters, one exported component per file) are
in the [`routes` docs](https://pkg.go.dev/github.com/nkapatos/templatic/experimental/routes).

## Files

- `pages/page.go`: `Page`, `PageOption`, `WithLayout`
- `pages/ctx.go`: `Ctx` and its accessors
- `pages/route.go`: `Route`, `ServeMuxPattern`, `PathWith`
- `routes/routes.go`: `Scan`, `Generate`, `Write` and the page conventions

## Tests

```sh
go test ./experimental/...
```

API docs:
[pages](https://pkg.go.dev/github.com/nkapatos/templatic/experimental/pages),
[routes](https://pkg.go.dev/github.com/nkapatos/templatic/experimental/routes).
