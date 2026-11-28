# 16 · client libraries

Front-end libraries from npm beside the server's live state, in one small
app. A release queue that every open page shares, reordered by dragging
(SortableJS) or with arrow buttons; a few numbers and times formatted in
the browser's locale (`Intl`); and a map of offices (Leaflet) that only
the map page loads.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

Open the queue in two windows and drag a task in one.

## Which pattern fits

| Library | Pattern | When |
|---|---|---|
| SortableJS | a hook on server markup, sending a typed command | the library adds behavior to markup the server renders and keeps live |
| Leaflet | a lit island that loads the library with `import()` | the library owns its DOM, and it is heavy or used on few pages |
| `Intl` | a hook that rewrites server-rendered text | pure presentation the browser knows better (locale, time zone), with the server's text as the fallback |

## What it shows

- **The server owns the order; the drag only proposes it.** The queue is
  one world region (`queue.go`). `client/sortable.ts` puts SortableJS on
  the region's `<ol>` with `ui.Hook("sortable", nil)`. On drop it sends
  the move with `send("queue.move", { id, to })` from
  `"templatic/commands"`, the typed module `generate` writes from the
  `live.NewCommand` declarations: a wrong command name or input field is
  a TypeScript error (`check-ts`), not a 400.
- **One command, both transports.** `Move` takes a task and its new
  position, not the whole order. The arrow buttons are a
  `ui.FormFor(Move, "/move")` per task whose submit buttons carry `to`,
  so the same command serves the drag, the arrows with JavaScript and
  the arrows without it. A move also merges with a concurrent move on
  another page, where a whole order would overwrite it. The server clamps
  `to`, so a page a redraw behind still lands somewhere valid.
- **The morph settles the order.** SortableJS has already moved the item
  when the command goes out, so the redraw that follows matches the DOM
  and the morph moves nothing (each `<li>` has an `id`, which idiomorph
  matches on). If the server's order differs, because another page moved
  something meanwhile, the morph wins: the page shows the server's order.
  A redraw that lands during a drag does the same, mid-drag: the morph
  puts the items in the server's order and strips SortableJS's classes,
  and the drop then moves the dragged item within that order. Both pages
  end in the same order. A refused move puts the items back
  (`sortable.sort` with the order at drag start), since no redraw comes.
  templatic has no way to hold a region's morph while a library holds
  its DOM; keeping the region small and its redraws rare keeps this
  rare.
- **The stats are formatted in the browser.** `components/home.templ`
  renders each value as `<data value="1234.50" data-currency="EUR">` or
  `<time datetime="…">` with plain server text inside (`1234.50 EUR`,
  `2026-11-27 08:38:51 UTC`) and `ui.Hook("intl", kind)`.
  `client/intl.ts` reads the attribute and writes the text with
  `Intl.NumberFormat`, `DateTimeFormat` or `RelativeTimeFormat`, in the
  browser's locale and time zone. The stats region is redrawn every two
  seconds; the morph puts the server's text back and the hook's
  `updated` formats it again in the same task, so the server text never
  paints. The relative time also refreshes every second until the hook
  is destroyed (`signal`).
- **Leaflet is its own chunk.** `client/islands/place-map.ts` calls
  `await import("leaflet")` in `firstUpdated`, so esbuild splits Leaflet
  into `.templatic/dist/chunks/leaflet-src-*.js` (about 150 KB, 43 KB
  gzipped). The chunk is not in `tc.json` and nothing preloads it; the
  island's module fetches it on mount. The queue page fetches neither the
  island nor Leaflet. Leaflet is a UMD module, so its API is the
  `default` of the imported namespace.
- **Leaflet's CSS comes with the island.** The define module imports
  `leaflet/dist/leaflet.css` statically, so it joins
  `islands/place-map-*.css` (11 KB), which the runtime links when the
  island mounts. Imported beside the lazy `import()` instead, it lands
  in that same file, plus an unused copy next to the chunk. The CSS's
  images are bundled as assets.
- **No tile server.** The map has no tile layer: circle markers with
  tooltips on a blank map, so it works offline and fetches nothing from
  another host. A real map adds `L.tileLayer` with a tile provider's URL
  and attribution. Circle markers also avoid Leaflet's default marker
  icon, whose image path the library guesses at run time.
- **The island's body is the fallback.** Without JavaScript the map page
  shows a table of the places.
- **Without JavaScript** the queue renders with its arrows and without
  the drag handles (a `<noscript>` style in `components/layout.templ`);
  an arrow POSTs to `/move` and redirects back. The stats show the
  server's text.
- **What it costs.** SortableJS ships in the app entry with the hooks,
  so every page loads it (about 39 KB, 14 KB gzipped, with `intl.ts`). A
  library only one page needs could go in a page module (`ui.Module`)
  instead.

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the `Move` handlers, the places, the mux and the asset handlers |
| `queue.go` | the queue and stats regions, `move`, the stats ticker |
| `components/queue.go` | `Task`, `MoveIn`, the `Move` command, `Stats` |
| `components/places.go` | `MapProps` and `Place` |
| `components/home.templ` | the queue page, the queue and stats regions |
| `components/map.templ` | the island pragma, its fallback table, the map page |
| `components/layout.templ` | the document, `Head`, `Boot` |
| `client/sortable.ts` | the hook that drags the queue and sends `queue.move` |
| `client/intl.ts` | the hook that formats values in the browser's locale |
| `client/islands/place-map.ts` | the lit element that loads Leaflet and draws the map |
| `client/app.ts`, `client/app.css` | the bundle's entry and stylesheet |
| `public/favicon.svg` | a file the app serves itself |

`*_templ.go` and `*_tc.go` files are generated by `templatic generate` and
committed. `.templatic/` is generated and ignored.

A page kept
open across two rebuilds can miss the Leaflet chunk
([#16](https://github.com/nkapatos/templatic/issues/16)).
