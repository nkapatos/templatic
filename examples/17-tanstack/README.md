# 17 · tanstack

TanStack's headless libraries in lit islands, through their Lit adapters:
a 10,000-row log in a virtual list (`@tanstack/lit-virtual`), fed by a
world cell the server adds rows to; an orders table sorted, searched and
paged in the browser (`@tanstack/lit-table`); and a sign-up form that
checks each field while typing (`@tanstack/lit-form`) and submits through
the generated typed command, whose `Validate` and handler still decide.
Each page renders a plain-HTML fallback that its island replaces.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

Open each page in two windows: press a rate button on the log, sort the
table differently in each, sign up in one and watch the taken names in
the other. Then turn JavaScript off and sign up again.

## What it shows

- **Each island's TanStack code loads only where it is used.** The
  adapters are ordinary dependencies in `package.json`, each imported by
  one define module in `client/islands/`, so each island's file carries
  its library: `log-list` about 38 KB (12 KB gzipped), `order-table`
  44 KB (14 KB), `signup-form` 57 KB (15 KB). lit (17 KB, 7 KB gzipped)
  and `@tanstack/store`, which Table and Form share, are chunks of their
  own, so each is loaded once whichever islands a page has. `Boot`
  preloads only the islands the page renders: `/` requests
  `log-list-*.js` and nothing of Table or Form. A page left open across
  two rebuilds can still miss a pruned chunk
  ([#16](https://github.com/nkapatos/templatic/issues/16)).
- **A virtual list over a bound cell.** `bind=rows:log.rows`
  (`components/log.templ`) pushes the whole log into `props.rows`.
  `client/islands/log-list.ts` holds one `VirtualizerController` and
  passes the new count with `setOptions` in `willUpdate`, as the adapter
  asks; `getItemKey` keys rows by id. Only the rows in view, about 26,
  are in the DOM. The fallback is the newest 20 rows.
- **A client table whose state stays in the browser.** The orders are a
  bound cell too (`bind=orders:orders.all`), and the server changes a few
  statuses every three seconds. `client/islands/order-table.ts` keeps
  one `TableController` and passes the current props as `data` on every
  render. Sort, search and page are table state, not props, so a push
  changes the rows and leaves each viewer where they were;
  `autoResetPageIndex: false` keeps the page. The features are imported
  one by one (`rowSortingFeature`, `sortedRowModel`, …), so the bundle
  carries only those. The fallback is every order in id order.
- **A client form, with the server deciding.**
  `client/islands/signup-form.ts` checks the same shapes as
  `SignupIn.Validate` (`components/signup.go`) on every change and shows
  an error once a field was touched. On submit it calls `send` from
  `templatic/commands`, typed by `SignupIn`. Only the server knows the
  reserved names (`Validate`) and the taken ones (the handler in
  `signup.go`); either refusal is a `live.Invalid` on `username`, which
  reaches the client as a `CommandError` with `fields`. The island hands
  them to `formApi.setErrorMap({ onSubmit: { form, fields } })`, so they
  show in the same field as the client's errors, and TanStack Form
  clears each one when its field next passes the change check.
- **The same command without JavaScript.** The island's body is a plain
  `ui.FormFor(Register, "/signup", failed)`, and `Register.FormPOST` in
  `main.go` takes its POST: a refusal renders the page again with 422,
  the errors in `ui.FieldError` and the typed values back; a success
  redirects to `/signup`. The refused input is not marked `aria-invalid`
  without JavaScript ([#59](https://github.com/nkapatos/templatic/issues/59)).
  The island's typed `send` is not a form submission, so the run-once
  guarantee for forms does not cover it; the island disables its button
  while a submit is in flight
  ([#63](https://github.com/nkapatos/templatic/issues/63)).
- **No `@tanstack/query-core`.** Fetching, caching and invalidation are
  what the live channel already does: the cells are the data, and every
  change is pushed. A query cache beside them would be a second source of
  truth for the same dataset, and keeping the two in step is the bug to
  avoid.
- **The server side is tested without a browser.** `main_test.go` checks
  that the log keeps 10,000 rows newest first, and both refusals of
  `Register` through `livetest`.

## What a whole-value cell costs

Values are published whole ([design.md](../../design.md#the-live-channel)):
every added row sends all 10,000 rows to every open page. Measured on a
5-core aarch64 Linux VM (8 GB), Go 1.27, headless Chromium 159, server
and browser on the same machine. One page on `/`, a second on `/signup`,
30 seconds per rate after a 2-second warm-up:

| Adds/s | Frame | Stream, `/` | Stream, `/signup` | Client busy per update | Bind, lit and virtual render | Client busy per second | Server CPU |
|---|---|---|---|---|---|---|---|
| 1 | 836 KB | 0.8 MB/s | 0.8 MB/s | 13.7 ms | 0.3 ms | 14 ms | 2% |
| 5 | 836 KB | 3.9 MB/s | 3.9 MB/s | 11.3 ms | 0.2 ms | 54 ms | 8% |
| 20 | 836 KB | 15.5 MB/s | 15.5 MB/s | 3.6 ms | 0.1 ms | 68 ms | 8% |

How it was measured: a Playwright script with a CDP session on each
page. The frame size is the `data` of `Network.eventSourceMessageReceived`
for `log.rows`; the stream rate is the sum of `Network.dataReceived` for
the `/live/stream` request. Client busy is the change in the
`TaskDuration` metric (`Performance.getMetrics`) over the run, less the
same at 0 adds/s, divided by the frames received. The bind column wraps
the island's `tcBind` and times it until lit's `updateComplete`. Server
CPU is the server process's user plus system time from `/proc`, in
percent of one core. Every frame was applied: none were coalesced away
at these rates.

Reading it:

- **The cost is the size of the dataset, not of the change.** A row is
  about 80 bytes of JSON; each update sends 836 KB, about 10,000 times
  that. Gzip would bring the frame to about 85 KB if the app compresses
  the stream, but it is still the whole log.
- **The island is not the cost.** The virtualizer re-renders only the
  rows in view: 0.1 to 0.6 ms per update. The rest of the client time is
  receiving and decoding the frame (`JSON.parse` of one frame alone is
  about 1.7 ms), and presumably collecting each replaced copy. Per
  update it falls at 20 adds/s, presumably because several messages
  arrive in one task; per second it grows with the rate.
- **Pages without the list pay the same.** `/signup` receives every
  `log.rows` frame although it renders no log
  ([#36](https://github.com/nkapatos/templatic/issues/36)), and every
  fresh stream begins with a snapshot of every world cell, so each page
  load also receives the whole log once, on top of the 840 KB of HTML
  that `/` carries as island props
  ([#8](https://github.com/nkapatos/templatic/issues/8)).
- **What would fix it** is a keyed collection with snapshot plus deltas
  ([#4](https://github.com/nkapatos/templatic/issues/4)): one row per
  added row, and a client `collection(topic)` the virtualizer reads from.
  This example does not build one.

## Client table or server table

[06-table](../06-table) pages, searches and sorts on the server, with the
view in the URL. This table ships every order and does it in the browser.

| | Client table (here) | Server table (06) |
|---|---|---|
| Dataset | small enough to ship and to send again on each change: hundreds to a few thousand rows | any size: the page carries one page of rows |
| Interaction | instant, no round trip | one command and one swap per click |
| Shareable URL | no: sort, search and page live in the island | yes: `?page=2&q=acme&sort=amount` |
| Without JavaScript | the fallback, one unsorted table of every row | every link and the search form work |
| Live updates | a bound cell pushes the dataset to every viewer, and each keeps their own view in the browser; every change costs the whole dataset (above) | `SessionRegion.Refresh` renders each viewer's own page on the server; a change costs one page of HTML per viewer |

A client table fits when the whole dataset is small and the interaction
is the product. A server table fits when the dataset is large, the view
should survive a link or a reload, or the page must work without
JavaScript. A client table could keep its view in the URL too, but that
is the island's own code.

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the goroutines, both transports of `Register`, the routes |
| `logfeed.go` | the log cells, `SetRate` and the goroutine that adds rows |
| `orders.go` | the generated orders and the goroutine that changes them |
| `signup.go` | the taken names and `Register`'s handler |
| `main_test.go` | the log and `Register` tested with `livetest` |
| `components/log.go`, `orders.go`, `signup.go` | props, inputs, `Validate` and the commands |
| `components/log.templ`, `orders.templ`, `signup.templ` | the island pragmas, their fallbacks and the pages |
| `components/layout.templ` | the document, `Head`, `Boot` |
| `client/islands/log-list.ts` | the virtual list |
| `client/islands/order-table.ts` | the client table |
| `client/islands/signup-form.ts` | the lit-form form and the server-error mapping |
| `client/app.ts`, `client/app.css` | the bundle's sources |
| `public/favicon.svg` | a file the app serves itself |

`*_templ.go` and `*_tc.go` files are generated by `templatic generate` and
committed. `.templatic/` is generated and ignored. `client/app.ts` only
imports the stylesheet, so the bundle has an empty app module that every
page preloads ([#60](https://github.com/nkapatos/templatic/issues/60)).
