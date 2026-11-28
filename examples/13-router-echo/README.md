# 13 · router (echo)

The app of [13-router-stdlib](../13-router-stdlib), routed by
[echo](https://echo.labstack.com) v5. Everything but `routes` in `main.go`
is the same; read 13-router-stdlib for what the app shows.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

## What differs from stdlib

- **Every handler goes through `echo.WrapHandler`.** The feature's
  handlers, `w.SSE()`, `w.Commands()` and the file servers are plain
  `http.Handler`s; WrapHandler turns each into an echo handler.
- **Path values come from WrapHandler.** echo keeps its params in
  `c.PathValues()`, not in the request; echo v5's `WrapHandler` copies
  them into `r.SetPathValue`, so `r.PathValue("id")` and the form
  decoding see `:id`. A handler
  called another way (`h.ServeHTTP(c.Response(), c.Request())`, which is
  what echo v4's `WrapHandler` does) sees no path value: the item page is
  a 404 for every id, and the like form decodes the `id` field instead,
  so a forged field picks the item.
- **The world's handlers are mounted one by one.** `World.Mount` takes a
  `*http.ServeMux`, so `routes` mounts `w.SSE()` on `GET /live/stream` and
  `w.Commands()` on `POST /live/commands` itself, under the base Boot's
  `Live: "/live"` names.
- **Routes use echo's syntax.** `/items/:id`, and `/static/*` and
  `/_templatic/*` for the prefixes; `http.StripPrefix` is the same.
- **The echo instance is the handler.** `templatic.App` wraps the
  `*echo.Echo` that `routes` returns. echo's response flushes through to
  the connection, so the live stream reaches the browser frame by frame.
- **Router defaults differ.** echo answers `HEAD /` with 405 and unknown
  paths with a JSON 404.

## Files

As in [13-router-stdlib](../13-router-stdlib#files); `main.go` holds the
echo wiring.
