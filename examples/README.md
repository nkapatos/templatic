# templatic examples

Small apps that build on each other, from plain pages to a production
setup. Each one is its own Go module and npm package: run it from its own
directory with the commands in its README.

| Example | Shows |
|---|---|
| [01-pages](01-pages) | static templ pages with bundled, fingerprinted assets; no live state |
| [02-live-value](02-live-value) | one world cell on every open page, changed by a server goroutine and by buttons (`ui.Cell`, `ui.CellAttr`, `ui.Button`) |
| [03-forms](03-forms) | one command served to JS and plain form posts: validation, field errors, 422 re-render, redirect after save, 404, run-once submissions |
| [04-url-state](04-url-state) | per-viewer state in the URL: tabs, search and the open item in a session region, swapped in place by `ui.Link` |
| [05-hooks](05-hooks) | client-only behavior with hooks: a menu, a cookie-backed theme, a modal dialog; hooks on swapped-in markup |

## Running one

You need Go and Node.js with npm. From an example's directory:

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

`ADDR=:9000 go run .` picks another address. While you change code, run
`npm run dev` instead: it rebuilds on save, restarts the server on `.go`
changes and reloads open pages.

The examples use the templatic checkout they sit in (a `replace ../..` in
each `go.mod`), so `go tool templatic` runs the checkout's CLI. An app of
your own requires a templatic release instead.
