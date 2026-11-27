# templui

A [templatic](https://github.com/nkapatos/templatic) app.

```sh
go mod tidy
npm install
go tool templatic generate   # or: mise run generate
go run .                     # http://localhost:8080
```

Develop with `mise run dev` (or `npm run dev`): it rebuilds on change,
restarts the server on `.go` changes and reloads open pages.
`mise run check` typechecks the client and vets the server.

Generated code lives in `.templatic/` (gitignored, safe to delete with
`go tool templatic clean`). Import it as `"templatic"`,
`"templatic/commands"` and `"templatic/islands/{tag}.base"`.
