# 09 · ticker

A board of live prices: 100 instruments by default, 50 price changes a
second in total, each cell colored by the direction of its last move.
Every open page shows the same board. A small load program opens many
streams against the running board and reports what they receive.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

`TICKER_SYMBOLS=200 TICKER_RATE=2000 TICKER_WINDOW=1ms go run .` changes
the board's size, its update rate and the coalescing window.

## What it shows

- **One cell per price, from a family.** `live.NewCellFamily(w, "px", "")`
  in `board.go` declares the prices, `Price.Get(sym)` the cell of one
  symbol (topic `px.T001`). A second family, `dir`, holds `"up"` or
  `"down"`; `AsOf` is a plain cell with the time of the last batch.
- **Keys become topics.** A family key is spliced into the topic, so it
  must match `[A-Za-z0-9_.-]+`: `BRK.B` or `7203.T` work, `EUR/USD`,
  `^GSPC` or `BF B` make `Get` panic
  ([#15](https://github.com/nkapatos/templatic/issues/15)). The symbols
  here are generated (`T001`…) so they always fit; an app whose keys come
  from data maps them first.
- **Direction is an attribute.** `ui.CellAttr(b.Dir.Get(sym), "data-dir")`
  in `components/home.templ` puts the direction on the row; `client/app.css`
  colors `li[data-dir="up"]` and `li[data-dir="down"]`. `class` can be bound
  too, but it replaces the whole attribute.
- **The feed is a goroutine.** `feed` in `board.go` wakes every 100ms and
  moves `rate/10` random prices, then stamps `AsOf`. It sets `dir` only when
  the direction flips: every `Set` is a frame to every viewer, even when the
  value did not change.
- **The coalescing window.** Each viewer's stream waits
  `live.WithCoalesceWindow` (8ms by default) after the first change it
  receives, collects what else arrives, keeps only the last value of each
  topic, and writes the rest. So a change reaches a viewer one window after
  it is published, and a symbol that moves twice inside the window sends one
  frame. With many symbols, few changes repeat inside a window, and frames
  per viewer stay close to publishes per second (see the numbers below).
  A larger window trades latency for fewer frames only when the same
  topics change again and again; a smaller one cuts latency.
- **The browser applies changes once per animation frame**, latest value
  per topic, so a busy board does not cost more DOM work than the screen
  can show.

## Measuring

`cmd/load` loads the page N times for N page tokens, opens N event streams,
and reports frames and bytes per second, how long the `AsOf` stamp took
from publish to read, and the server's CPU and memory from `/proc` (Linux
only). Run the server as a built binary, so `-pid` names the server and
not `go run`:

```sh
go build -o ticker . && ./ticker &    # ticker is gitignored
go run ./cmd/load -n 500 -d 20s -pid $!
```

Frames per viewer follow the feed: at 50 price moves a second about 49
move (a zero step is skipped), about half flip a direction and `AsOf`
adds 10, so about 82 frames a second, about 75 bytes each.

Measured before [#41](https://github.com/nkapatos/templatic/pull/41)
(one flush per batch instead of per frame: about 13–22% less server CPU)
on a 5-core aarch64 Linux VM (8 GB), server and load program on
the same machine over loopback, 10-second runs after a 3-second warm-up,
Go 1.27. Latency is p50 / p99 from publish of `AsOf` to the load program
reading it; CPU is the server's, in percent of one core.

| Symbols | Changes/s | Window | Viewers | Frames/s per viewer | Frames/s total | MB/s total | Latency p50 / p99 | Server CPU | Server RSS |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 100 | 50 | 8ms | 1 | 84 | 84 | 0.006 | 9.2 / 10.4 ms | 1% | 10 MB |
| 100 | 50 | 8ms | 100 | 82 | 8.2k | 0.6 | 10.8 / 14.8 ms | 11% | 29 MB |
| 100 | 50 | 8ms | 500 | 81 | 41k | 3.1 | 14.8 / 21.9 ms | 46% | 87 MB |
| 100 | 50 | 8ms | 1000 | 81 | 81k | 6.2 | 15.7 / 26.3 ms | 84% | 153 MB |
| 100 | 50 | 8ms | 2000 | 83 | 166k | 12.5 | 12.2 / 27.1 ms | 112% | 317 MB |
| 100 | 50 | 1ms | 1 | 83 | 83 | 0.006 | 1.7 / 2.5 ms | 1% | 10 MB |
| 100 | 50 | 1ms | 500 | 84 | 42k | 3.2 | 10.6 / 15.8 ms | 51% | 104 MB |
| 200 | 500 | 8ms | 100 | 665 | 66k | 4.8 | 14.8 / 22.0 ms | 49% | 32 MB |
| 200 | 500 | 8ms | 500 | 702 | 351k | 25.5 | 14.3 / 32.5 ms | 120% | 116 MB |
| 200 | 2000 | 8ms | 100 | 2059 | 206k | 15.1 | 16.4 / 30.9 ms | 95% | 38 MB |
| 200 | 2000 | 8ms | 500 | 2338 | 1.17M | 85.7 | 19.7 / 42.1 ms | 158% | 140 MB |
| 200 | 2000 | 50ms | 500 | 2032 | 1.02M | 74.4 | 61.9 / 82.6 ms | 131% | 131 MB |
| 10 | 2000 | 8ms | 100 | 360 | 36k | 2.6 | 9.1 / 16.9 ms | 56% | 31 MB |
| 10 | 2000 | 50ms | 100 | 210 | 21k | 1.5 | 46.6 / 53.3 ms | 48% | 31 MB |

The load program itself used about a third of the server's CPU in each
run, on the same cores.

Reading it:

- **The window sets the floor of latency.** With one viewer, latency is the
  window plus about 1ms. With many viewers it grows, as the streams'
  goroutines share the cores to write their frames.
- **Coalescing pays off only for repeats.** With 200 symbols, frames per
  viewer track changes per second whatever the window; with 10 symbols at
  2000 changes a second, the same topics repeat and a 50ms window cuts
  frames from 360 to 210 a viewer, for 50ms of latency.
- **Server cost follows frames written.** Each frame is its own write and
  flush to the socket, so the window gathers frames but does not save
  writes. A CPU profile at 500 viewers and 2000 changes a second: 43% of
  the server's time in the per-frame flush (the `write` system call), 11%
  formatting frame headers, 9% setting the write deadline per frame, 8%
  in the coalescer, 6% handing changes to the sessions.
- **Memory grows by about 150 KB of RSS per open stream**; about 65 KB of
  it is the stream's 1024-event buffer. A viewer that falls 1024 events
  behind is sent a resync and reloads; none did in these runs.
- **A publish is cheap to fan out**: one `Set` hands the change to every
  session in turn, about 60ns each in the profile, on the goroutine that
  called `Set`.
- **A browser keeps up**: one headless Chromium page with 200 symbols at
  2000 changes a second spent about a quarter of its main thread on
  script, with no long tasks and steady 60fps frames.

Racing writers to one cell can leave a viewer on an older value
([#13](https://github.com/nkapatos/templatic/issues/13)). Here one goroutine
writes every cell, so publishes to a topic never race; the load program
checks that `AsOf` never goes backwards on any stream.

## Files

| File | Holds |
|---|---|
| `main.go` | the world and its options, the feed goroutine, the mux and the asset handlers |
| `board.go` | the cell families and the feed loop |
| `components/board.go` | `Board`: the symbols and their cells |
| `components/home.templ` | the board page |
| `components/layout.templ` | the document, `Head`, `Boot` |
| `client/app.ts`, `client/app.css` | the bundle's sources |
| `cmd/load/main.go` | the load program |

`*_templ.go` files are generated by `templatic generate` and committed, as
with templ. `.templatic/` is generated and ignored.
