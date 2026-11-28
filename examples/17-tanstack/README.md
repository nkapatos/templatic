# 17 · tanstack

TanStack's headless libraries in lit islands, through their Lit adapters:
a 10,000-row log in a virtual list (`@tanstack/lit-virtual`), fed by a
data collection the server adds rows to; an orders table sorted, searched and
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
- **A virtual list over a bound collection.** The log is a
  `live.NewCollection` without `Renders`, so its changes travel as JSON
  items. `LogProps.Rows` is a `live.Snapshot[Row]` (`coll.Snapshot()`),
  whose TypeScript type is `Row[]`; `bind=rows:log.rows`
  (`components/log.templ`) makes the page hold the log from that
  snapshot, receive each added row as one op, and hand `props.rows` a new
  array once per frame. `Limit(LogSize)` drops the oldest row on the
  server and on the page alike, so a drop is not sent.
  `client/islands/log-list.ts` holds one `VirtualizerController` and
  passes the new count with `setOptions` in `willUpdate`, as the adapter
  asks; `getItemKey` keys rows by id. Only the rows in view, about 26,
  are in the DOM. The fallback is the newest 20 rows.
- **A client table whose state stays in the browser.** The orders are a
  bound collection too (`bind=orders:orders.all`), and the server changes
  a few statuses every three seconds with `Update`, one op per order. `client/islands/order-table.ts` keeps
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
  `ui.FormFor(Register, "/signup", failed)`, and `Register.Form` in
  `main.go` takes its POST: a refusal renders the page again with 422,
  the errors in `ui.FieldError` and the typed values back; a success
  redirects to `/signup`. `ui.FieldAttrs` marks the refused inputs
  `aria-invalid` without JavaScript, as the island does with it.
  The island runs its signup once however often it is submitted: it keeps
  a key from `submitKey()` and passes it as `send(…, { once: key })`
  until a submit succeeds, so a repeat gets the first outcome.
- **No `@tanstack/query-core`.** Fetching, caching and invalidation are
  what the live channel already does: the collections are the data, and
  every change is pushed. A query cache beside them would be a second source of
  truth for the same dataset, and keeping the two in step is the bug to
  avoid.
- **The server side is tested without a browser.** `main_test.go` checks
  that the log keeps 10,000 rows newest first, that a new row and an order
  change each reach a page as one op, and both refusals of `Register`
  through `livetest`.

## What a change costs

Before [#4](https://github.com/nkapatos/templatic/issues/4) the log was a
cell, and a cell is published whole: every added row sent all 10,000 rows
to every open page. As a collection, an added row is one op of about 165
bytes, sent only to pages that show the log. Both versions measured with
the same script, on a 5-core aarch64 Linux VM (8 GB) on Apple Silicon,
Go 1.27, headless Chromium 159, server and browser on the same machine.
One page on `/`, a second on `/signup`, 30 seconds per rate after a
2-second warm-up:

| Adds/s | | Frame | Stream, `/` | Stream, `/signup` | Client busy per frame | Client busy per second | Server CPU |
|---|---|---|---|---|---|---|---|
| 1 | cell | 833 KB | 848 KB/s | 848 KB/s | 4.97 ms | 5.0 ms | 0.5% |
| 1 | collection | 166 B | 191 B/s | 0 | 1.76 ms | 1.8 ms | 0.0% |
| 5 | cell | 834 KB | 4.1 MB/s | 4.1 MB/s | 4.16 ms | 20.5 ms | 2.1% |
| 5 | collection | 166 B | 951 B/s | 0 | 1.25 ms | 6.2 ms | 0.1% |
| 20 | cell | 835 KB | 15.9 MB/s | 15.9 MB/s | 3.88 ms | 73.9 ms | 7.3% |
| 20 | collection | 165 B | 3.7 KB/s | 0 | 0.93 ms | 18.1 ms | 0.3% |

How it was measured: a Playwright script with a CDP session on each
page. The frame size is the `data` of `Network.eventSourceMessageReceived`
for `log.rows`; the stream rate is the sum of `Network.dataReceived` for
the `/live/stream` request. Client busy is the change in the
`TaskDuration` metric (`Performance.getMetrics`) over the run, less the
same at 0 adds/s, divided by the frames received. Server CPU is the
server process's user plus system time from `/proc`, in percent of one
core. A loop kept one other core busy throughout: without it the host
lowers the clock of a nearly idle VM, and the collection's page, idle
between frames, measured each layout about five times slower than the
cell's page, which kept the CPU busy parsing.

Reading it:

- **The cost is the size of the change, not of the dataset.** The frame
  is one row, about 5,000 times smaller than the whole log was, and the
  stream at 20 adds/s is 3.7 KB/s where it was 15.9 MB/s.
- **Pages without the list pay nothing.** `/signup` shows no log, so it
  does not subscribe to `log.rows` and receives none of it, as it would
  receive none of a cell it does not show.
- **The client time left is the list's own.** The island re-renders only
  the rows in view; the per-frame time is that render and the browser's
  layout of it, about 1 ms. Decoding an 834 KB frame is gone.
- **The page load is a little larger.** The snapshot in the island's
  props carries each row's key beside it: `/` is 1,019 KB of HTML, 840 KB
  with the cell.

## Client table or server table

[06-table](../06-table) pages, searches and sorts on the server, with the
view in the URL. This table ships every order and does it in the browser.

| | Client table (here) | Server table (06) |
|---|---|---|
| Dataset | small enough to ship with the page: hundreds to a few thousand rows | any size: the page carries one page of rows |
| Interaction | instant, no round trip | one command and one swap per click |
| Shareable URL | no: sort, search and page live in the island | yes: `?page=2&q=acme&sort=amount` |
| Without JavaScript | the fallback, one unsorted table of every row | every link and the search form work |
| Live updates | a bound collection sends each change to the viewers that show it, and each keeps their own view in the browser; a change costs the changed rows (above) | `SessionRegion.Refresh` renders each viewer's own page on the server; a change costs one page of HTML per viewer |

A client table fits when the whole dataset is small and the interaction
is the product. A server table fits when the dataset is large, the view
should survive a link or a reload, or the page must work without
JavaScript. A client table could keep its view in the URL too, but that
is the island's own code.

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the goroutines, both transports of `Register`, the routes |
| `logfeed.go` | the log collection, the rate cell, `SetRate` and the goroutine that adds rows |
| `orders.go` | the generated orders and the goroutine that changes them |
| `signup.go` | the taken names and `Register`'s handler |
| `main_test.go` | the log, the orders and `Register` tested with `livetest` |
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
