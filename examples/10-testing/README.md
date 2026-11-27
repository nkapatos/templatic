# 10 · testing

How an app tests its live pages. The app is small: a count every open page
shares, a tally of the adds made from this page, and a log of every add and
of a server clock tick every 20 seconds. Its tests come in two layers: Go
tests with `live/livetest`, which run commands against in-memory sessions,
and a browser check, which runs the built app in Chromium.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
go test ./...                # the livetest tests
npx playwright install chromium
npm run check-browser        # the browser check
```

## What it shows

- **The app under test.** `newCounter` in `counter.go` declares the `count`
  cell and the `log` collection on a `live.World`, and registers `Add`'s
  handler. `main.go` calls the same function on its world, so a test builds
  exactly what the app serves, without HTTP. The tally is a session region
  (`//templatic:region` on `Tally` in `components/home.templ`); `Add`
  swaps it for the session that called it.
- **Commands.** `livetest.Call(s, components.Add, components.AddIn{N: 2})`
  runs the command through its input decoding, `Validate` and handler.
  `livetest.Submit` does the same with form fields, as the runtime sends a
  form: `"4"` decodes into the `int` field. A refused input comes back as
  an error that `live.InputErrors` lists, and sends nothing
  (`TestAddRefusesABadStep`).
- **Cells.** `s.Cell(components.CountTopic, &got)` decodes the value the
  session holds. Two `livetest.New` sessions on one world are two pages:
  an add on one sets the cell on both (`TestAddReachesEveryPage`).
- **Swaps.** `s.Swaps(topic)` and `s.LastSwap(topic)` return the HTML
  swapped into a region. The tally swap reaches only the caller: the other
  session has none (`TestAddSwapsOnlyTheCallersTally`).
- **Collections.** A page gets a collection's changes once it shows it:
  `s.Show(c.log.Topic(), c.log.Version())` is a page that rendered the log
  as it stands. `s.Ops(topic)` then returns each change, its HTML with the
  item's root element included.
- **Waiting.** A command's events are recorded by the time `Call` returns.
  An update with no command behind it, such as `tick` from a goroutine,
  arrives later: `s.WaitOp(topic, substr, d)` and `s.WaitSwap` poll for
  it and fail the test after `d` (`TestTickLogsFromAGoroutine`).
- **Session end.** A livetest session is retired when its test ends, and
  the world's `OnDisconnect` runs then. A subtest scopes one page, so a test
  can check what the app does when a page leaves
  (`TestAPageTallyEndsWithItsSession`).
- **Only the standard `testing` package**, no assertion library.

## The browser check

`browser/check.cjs` is a plain Node script on the `playwright` library,
declared as a devDependency; `npx playwright install chromium` fetches the
browser it drives. `npm run check-browser` (or `mise run check-browser`)
builds the app with `go build` into a temporary directory, runs it on a
free port (`PORT=8110` sets one), checks it and stops it. Run
`go tool templatic generate` first: the check uses the generated bundle as
it is. It exits non-zero on the first failed check. It is not a Go test, so
`go test ./...` does not need Node or a browser.

What it checks:

- **Two pages with JavaScript.** Page `a` loads through a relay, page `b`
  directly. An add on `a` shows on both, the tally only on `a`, the log
  line on `b`.
- **`window.tcState()`**, the runtime's self-report. The check waits for
  `live.connection === "open"` on both pages before it acts: an update
  sent between a page's render and its live connection does not reach that
  page ([#39](https://github.com/nkapatos/templatic/issues/39)). After the
  reconnect it checks `live.disconnects` and `live.resumes`, and at the end
  that `lastErrors` is empty.
- **A dropped connection.** The browser's offline mode does not close an
  EventSource that is already open, so `browser/relay.cjs` is a TCP relay
  in front of the app: `cut()` closes every connection through it and
  refuses new ones, `restore()` lets them through again. With `a` cut off
  the check adds on `b`, restores, and checks that `a` resumes within the
  grace with the current count and the entries it missed.
- **Without JavaScript.** A third page with JavaScript off posts the form:
  the server redirects to the page with the new count, the live pages
  follow, and a refused step answers 422 with the field error.
- **Console errors fail the check**, and so do uncaught exceptions. The
  browser logs a failed load as a console error too; the two the check
  causes on purpose (the cut connection on `a`, the refused post's 422) are
  expected by their URL, any other one fails.

## Reconnects in livetest and in the browser

`a.Drop()` detaches a session as a page losing its connection does, and
`a.Resume()` reattaches it within the grace: the test then checks that `a`
gets the log entries it missed. `livetest`'s `Cell` reads the world's
current value, not what a page received, so that a resumed page gets the
count again is checked only in the browser.

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the tick goroutine, the page, the no-JS post, the routes |
| `counter.go` | the count, the log, each page's tally, `Add`'s handler, `add` and `tick` |
| `counter_test.go` | the livetest tests |
| `browser/check.cjs` | the browser check |
| `browser/relay.cjs` | the TCP relay that cuts and restores connections |
| `components/counter.go` | `AddIn`, `Add`, `Entry`, `PageAdds` and the topics |
| `components/home.templ` | the page, the tally region, the log line |
| `components/layout.templ` | the document |
| `client/app.ts`, `client/app.css` | the styles |

`*_templ.go` and `*_tc.go` files are generated by `templatic generate` and
committed, as with templ. `.templatic/` is generated and ignored.
