# 14 · bootstrap

Bootstrap 5 from npm in a templatic app: a live status board of cards,
badges and button groups, with Bootstrap's dropdowns, collapse, modal,
tooltips and popovers. The server measures a random service's latency
every five seconds; anyone can change a service's status or report an
incident, and every open page shows the same board.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

Open it in two windows and press a status button in one.

## What it shows

- **Bootstrap is an npm dependency, bundled like the app's own code.**
  `bootstrap` and its peer `@popperjs/core` are in `package.json`;
  `client/app.ts` imports `bootstrap/dist/css/bootstrap.css` and the
  hooks import `Tooltip`, `Popover` and `Modal` from `"bootstrap"`. The
  CSS joins the app entry's stylesheet, so `Head` links the
  fingerprinted file; nothing comes from a CDN. Sass is not needed: the
  compiled CSS is imported as it is. To theme with Sass or purge unused
  rules, run that tool as an npm script and import its output.
- **What it costs.** The app module carries all of Bootstrap's JS and
  Popper, once (about 82 KB, 25 KB gzipped); the boot module and the
  shared chunk carry the runtime only. The stylesheet is the whole of
  Bootstrap (about 230 KB, 31 KB gzipped). Bootstrap's ESM build wires
  its data API when it is imported, so it does not tree-shake.
- **Data-API widgets work on live markup.** Importing Bootstrap
  installs its dropdown, collapse and modal handlers on `document`, so
  the dropdown in each card (`components/board.templ`) works on markup a
  redraw brought in, with no hook. A redraw closes an open dropdown: see
  below.
- **Tooltips and popovers need a hook.** They have no data API: each
  element needs an instance. `client/tooltips.ts` defines `bs-tooltip`
  and `bs-popover`; `mounted` creates the instance, `destroyed` disposes
  it. Disposing matters: the incident badge leaves the card when the
  service is set back to up, and a tooltip that is showing at that
  moment would stay on the page without it. The titles are in
  `data-bs-title`, not `title`, because Bootstrap moves a `title` into
  another attribute and the next morph would put it back. A redraw that
  changes `data-bs-title` calls the hook's `updated`, which sets the new
  content on the existing instance.
- **Stateful widgets go outside swapped regions.** Bootstrap keeps a
  widget's state in the DOM (`.show`, `aria-expanded`, inline styles)
  and, for a modal, also in its instance and in a backdrop and a class on
  `<body>`. A morph makes the region's markup match the server's again,
  so it removes that state. In the board an open dropdown closes on the
  next redraw, which is fine for a menu that is open a moment. The
  legend collapse and the report modal are in `components/pages.templ`,
  outside the board region, and stay open across redraws. Tried inside
  the region, a collapse closed on the next redraw, and an open modal
  lost `.show` and its `display` while the backdrop and
  `body.modal-open` stayed: the page was covered and scroll-locked, and
  neither Escape nor a click closed it. A trigger may still be inside
  the region: the "Report incident…" item in each card's dropdown opens
  the modal outside it.
- **The modal's form is a templatic form.** `ui.FormFor(Report,
  "/report")` inside the modal raises `incident.report`; a refusal shows
  its field errors in the modal. `client/modal.ts` preselects the
  service from the trigger's `data-service` (`show.bs.modal`) and hides
  the modal and resets the form when the command succeeds.
  `client/app.css` gives templatic's error markup (`aria-invalid`,
  `[data-tc-field-error]`) Bootstrap's invalid look: `ui.FieldError` takes
  no class ([#54](https://github.com/nkapatos/templatic/issues/54)).
- **A button group is one form.** Each card's status buttons are submit
  buttons of one `ui.FormFor(Set, "/set")` with `name="status"`, so the
  pressed button's value is the input, with JS (the runtime sends the
  submitter) and without.
- **The board is one world region.** `w.Region("board", …)` in
  `board.go`; every change calls `Redraw`, and every open page morphs
  the cards in place.
- **Without JavaScript** the page renders styled, the status buttons and
  the report form post and redirect back, and a refused report renders
  `/report` again with 422 and the errors. What degrades: dropdowns do
  not open (their items are also reachable through the Report link), the
  "Report incident" button opens the `/report` page instead of the modal,
  and tooltips and the About popover do not show. Collapses cannot open,
  so a `<noscript>` style in `components/layout.templ` shows their
  content.

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the command handlers, the mux and the asset handlers |
| `board.go` | the services, the board region, the latency ticker |
| `components/board.go` | `Service`, the `Set` and `Report` commands and their input |
| `components/board.templ` | the board region: cards, badges, dropdowns, button groups |
| `components/pages.templ` | the board page with the collapse and the modal, the report page |
| `components/layout.templ` | the document, the navbar, `Head`, `Boot` |
| `client/app.ts`, `client/app.css` | the bundle's entry and stylesheet |
| `client/tooltips.ts`, `client/modal.ts` | the hooks around Bootstrap's JS |

`*_templ.go` files are generated by `templatic generate` and committed;
`.templatic/` is generated and ignored.

The forms in the board region lose their run-once submission id on the
first redraw ([#32](https://github.com/nkapatos/templatic/issues/32)).
