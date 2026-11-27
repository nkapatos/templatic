# templatic examples

Small apps that build on each other, from plain pages to a production
setup. Each one is its own Go module and npm package: run it from its own
directory with the commands in its README.

| Example | Shows |
|---|---|
| [01-pages](01-pages) | static templ pages with bundled, fingerprinted assets; no live state |
| [02-live-value](02-live-value) | one world cell on every open page, changed by a server goroutine and by buttons (`ui.Cell`, `ui.CellAttr`, `ui.Button`) |
| [03-forms](03-forms) | one command served to JS and plain form posts: validation, field errors, 422 re-render, redirect after save, 404, run-once submissions |
| [04-url-state](04-url-state) | per-viewer state in the URL: tabs and the open item in a session region, swapped in place by `ui.Link`; a native GET search form outside it, with a hook |
| [05-hooks](05-hooks) | client-only behavior with hooks: a menu, a cookie-backed theme, a modal dialog; hooks on swapped-in markup |
| [06-table](06-table) | a server-driven table: paging, search and sort in the URL; every viewer refreshed on new data with their own view (`SessionRegion.Refresh`) |
| [07-feed](07-feed) | a live feed with streams: chat rooms on a `StreamFamily` appended at the bottom, an activity log on top, bounded history, reconnects |
| [08-island-chart](08-island-chart) | a Chart.js chart in a lit island, fed by bound world cells; the library loads only where the island is used |
| [09-ticker](09-ticker) | many values at a high rate to many viewers: cell families, coalescing, and a load program that measures it |
| [10-testing](10-testing) | how an app tests its live pages: `livetest` for commands, cells, swaps and appends; a Playwright check with a TCP relay for reconnects |
| [11-auth](11-auth) | login, per-user pages and admin-only commands: the user in the page token (`s.Params()`), an allowlisted cookie on commands, `http.CrossOriginProtection`, logout as a POST |
| [12-production](12-production) | the dev loop and shipping: gzip that skips the stream, no-cache HTML, h2c behind a TLS proxy, timeouts above the heartbeat, `X-Forwarded-For`, graceful shutdown, sticky replicas, a Dockerfile |
| [13-router-stdlib](13-router-stdlib) | one small app with a path value (`/items/{id}`), a cell family and a command, on `net/http`'s `ServeMux` (`World.Mount`); a feature package that hands out plain handlers |
| [13-router-chi](13-router-chi) | the same app on chi: `SSE()` and `Commands()` mounted by hand |
| [13-router-echo](13-router-echo) | the same app on echo v5: handlers through `echo.WrapHandler`, which sets the path values |
| [14-bootstrap](14-bootstrap) | Bootstrap from npm through the bundle: data-API widgets across morphs, tooltips and popovers in hooks, stateful widgets outside live regions |
| [15-tailwind](15-tailwind) | Tailwind as the app's own build step beside `templatic dev`: scanning `.templ` only, classes used only in live markup |
| [16-client-libraries](16-client-libraries) | client libraries beside live state: a SortableJS reorder through the typed generated commands, Leaflet loaded lazily in an island, locale formatting in a hook |
| [17-tanstack](17-tanstack) | TanStack Virtual, Table and Form in lit islands: a 10k-row bound list and what a whole-value cell costs, a client-side table, a lit-form form with the server's field errors |
| [18-templui](18-templui) | templUI (now shadcn-templ) components copied by its CLI: its own script bundle from `/static/`, its Tailwind theme through the bundle, tabs in a world region across morphs |

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
