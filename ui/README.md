# ui

The templ components and attribute builders that bind server-rendered markup
to a `live.World`. They emit `data-tc-*` markers that the client runtime
(started by `templatic.Boot`) reads to update elements and raise commands.

API documentation: https://pkg.go.dev/github.com/nkapatos/templatic/ui

## Neighbours

- `live` declares the cells, regions, streams and commands these components
  take as arguments.
- The root package provides `Head` and `Boot`, which load the client runtime
  that these markers depend on.
- Used from `.templ` files, for example `@ui.Cell(count)`.

## Files

| File | Holds |
| --- | --- |
| `cell.go` | `Cell`, `CellAttr`, `Bind`, `BindAttr`, deprecated `SessionCell` and `SessionCellAttr` |
| `region.go` | `Region` |
| `stream.go` | `Stream`, `StreamItem`, `Tail` |
| `action.go` | `Button`, `Link`, `Swaps`, deprecated `Action` and `ActionLink` |
| `form.go` | `FormFor`, `Multipart`, `Failed`, `FieldError`, `FormError`, `FieldValue`, `FieldValues`, deprecated `Form` |
| `hook.go` | `Hook` |
| `island.go` | `Island` |
| `module.go` | `Module` |
| `option.go` | `Option`, `Class`, `Attrs` and the shared element configuration |
| `doc.go` | the package documentation |

## Tests

```sh
go test -race ./ui/...
```

Tests render components and check the markup. `mise run test` runs the whole
module.
