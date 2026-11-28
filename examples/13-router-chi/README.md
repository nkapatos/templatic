# 13 · router (chi)

The app of [13-router-stdlib](../13-router-stdlib), routed by
[chi](https://github.com/go-chi/chi) v5. Everything but `routes` in
`main.go` is the same; read 13-router-stdlib for what the app shows.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

## What differs from stdlib

- **The world's handlers are mounted one by one.** `World.Mount` takes a
  `*http.ServeMux`, so `routes` mounts `w.SSE()` on `GET /live/stream` and
  `w.Commands()` on `POST /live/commands` itself: the paths Mount would
  use, under the base Boot's `Live: "/live"` names.
- **Path values need nothing.** chi sets `r.SetPathValue` for its URL
  params, so `r.PathValue("id")` in the handlers and the form decoding
  see `{id}` as with the ServeMux. `TestLikeFormUsesThePathID` checks it.
- **Prefixes are wildcards.** `/static/*` and `/_templatic/*` instead of
  the ServeMux's trailing slash; `http.StripPrefix` is the same.
- **The chi router is the handler.** `templatic.App` wraps the
  `*chi.Mux` that `routes` returns, as it wraps a ServeMux; the live
  stream reaches the browser frame by frame through it.
- **Router defaults differ.** chi answers `HEAD /` with 405 (the ServeMux
  serves HEAD with the GET handler) and has its own 404 for unknown paths.

## Files

As in [13-router-stdlib](../13-router-stdlib#files); `main.go` holds the
chi wiring.
