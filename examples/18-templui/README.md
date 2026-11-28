# 18 · templui

A shared task list built from [templUI](https://shadcn-templ.com)
components: card, badge, button, input and field for the static parts, a
dialog outside the live region, and tabs inside it. Any open page can add
a task or mark one done, and every page redraws the list.

```sh
npm install
npm run generate             # Tailwind, templUI's scripts, then templatic generate
go run .                     # http://localhost:8080
```

Open it in two windows, open the Done tab in one and add a task in the
other.

## templUI is now shadcn-templ

templUI 2.0 was renamed: the Go module is
`github.com/axadrn/shadcn-templ/v2` and the CLI is `shadcn-templ`. The old
`github.com/templui/templui` (v1.13.4) is marked deprecated in its
`go.mod`, and `go get` prints that notice. This example uses the current
release, v2.0.1. The directory and the module keep the name templui.

## Installing it

templUI has two workflows: importing its packages as a Go module, or a CLI
that copies the component source into the app, shadcn-style. This example
uses the CLI, the workflow templUI's installation guide leads with:

```sh
go get -tool github.com/axadrn/shadcn-templ/v2/cmd/shadcn-templ@v2.0.1
go tool shadcn-templ init --css tailwind.css --preset nova
go tool shadcn-templ add button card badge input field dialog tabs
```

- **The CLI is pinned in `go.mod`'s `tool` block**, not installed
  globally, as templUI recommends; `go tool shadcn-templ` runs that
  version. It adds its own dependencies to the app's `go.mod`.
- **The components are the app's code.** `add` writes each one as a
  package under `components/` (`components/button`, `components/dialog`,
  …), with the dependencies it resolves (`card` pulls in `aspectratio`,
  `dialog` pulls in `icon`). It also writes `utils/` and
  `components/scripts.templ`. They are committed and the app owns them;
  `add --overwrite` updates them.
- **The CLI pins the CLI, not the components.** `add` fetches components
  from the registry at shadcn-templ.com, so the version is whatever the
  registry served on the day of the `add`. The source copied into the app
  is what pins them.
- **The CLI copies `.templ` files only.** `templatic generate` runs templ's
  generator over them like over the app's own, so their `*_templ.go` files
  are generated and committed the same way.
- **Pass IDs in markup that redraws.** Dialog, tabs and inputs fall back
  to `utils.RandomID()`, so every render of the same markup has new ids.
  The tasks region passes fixed ids (`task-tabs`, `tab-open`, …).

## Tailwind

The same setup as [15-tailwind](../15-tailwind): an npm script compiles
`tailwind.css` to `client/tailwind.out.css`, which `client/app.ts`
imports, so templatic bundles, fingerprints and links it.

- `shadcn-templ init --css tailwind.css` merged templUI's theme (the CSS
  variables, `@theme inline`, the base layer) into the existing file and
  vendored `tw-animate.css` and `shadcn-tailwind.css` next to it. It kept
  `source(none)`.
- `@source "./components/**/*.templ"` covers the app's templates and the
  copied components, which are in the same tree. With the import workflow
  they would be in the module cache, and the glob would have to name that
  directory.
- The app entry holds only CSS, so `generate` also emits an empty JS
  file that `Head` preloads
  ([#60](https://github.com/nkapatos/templatic/issues/60)).
- The `nova` preset expects the Geist font, which the CLI does not
  install. This example does not load it, so the page uses the system
  sans-serif font.

## templUI's JavaScript

templUI's scripts are classic scripts, not modules. `shadcn-templ
bundle` concatenates every `components/*/*.js` in a fixed order (the
shared `baseui/` helpers first), minifies the result and writes
`public/js/shadcn-templ-<hash>.js`. It also writes the URL into
`components/scripts_bundle.go`, and `@Scripts()` in
`components/layout.templ` renders the one `<script defer>` tag.

- **Served from `/static/js/`, not through templatic's bundle.**
  `components.json` sets `scripts.dir` to `public/js` and `scripts.path`
  to `/static/js`, so the app's own `/static/` handler serves it. The files
  share top-level names and depend on load order, so importing them from
  `client/app.ts` as modules would break them. The file name is already a
  content hash, so it needs no templatic fingerprint.
- **Built by the npm scripts.** `generate` runs `shadcn-templ bundle`
  before templatic; `dev` runs `shadcn-templ bundle --watch` as a third
  watcher (readable output, same file name), as templUI's own dev setup
  does. A changed component script rewrites `scripts_bundle.go` with a new
  hash, so `templatic dev` restarts the app and open pages reload. The
  `.js` output is ignored by git, and `scripts_bundle.go` is committed.
- **What it costs.** For these seven components the bundle is about 45 KB
  (15 KB gzipped). `components/icon` carries every Lucide icon as Go data
  (about 350 KB of source).
- **It starts itself on live markup.** `baseui/lifecycle.js` runs one
  `MutationObserver` on the document: a component is set up when its
  element appears and torn down when the element leaves. Clicks and keys
  are handled by listeners on `document`. A templatic redraw that morphs
  an element in place keeps it, so its setup stays. A new element is set
  up by the observer. No hook is needed. templUI has no per-component
  init function; `window.templ.lifecycle.register` is the only entry
  point.

## Widget state across redraws

A redraw makes the region's markup match the server's again (see
[14-bootstrap](../14-bootstrap)), so any state templUI's script wrote into
the DOM is reset. The tabs show it: open the Done tab, and the next redraw
selects Open again, because the server renders Open as the default. The
tabs still work after the redraw, including arrow-key focus.

The task list is one world region shared by every viewer, so the server
cannot render each viewer's tab. A per-viewer tab would need a session
region with the tab in the URL ([04-url-state](../04-url-state)).

The About dialog is outside the region and stays open across redraws.
Tried inside the region, an open dialog or dropdown menu closed cleanly
on the next redraw. templUI moves an open overlay to `<body>`, and when
the morph puts the overlay's markup back in place, the lifecycle unmounts
the moved one and releases the scroll lock and `inert`. The widget opens
again afterwards. Put overlays that must stay open outside live regions.

## The form

`ui.FormFor(Add, "/add")` wraps a templUI `field.Field`, `field.Label`,
`input.Input` and a submit `button.Button`; `ui.FieldValue` fills the
input's `Value`.

- **Field errors.** `ui.FieldError("title")` sits in a span with
  `text-destructive`; `ui.FieldError` takes no class
  ([#54](https://github.com/nkapatos/templatic/issues/54)). templUI's
  `field.Error` is not used: it always renders a `role="alert"` element,
  so the form would carry an empty alert.
- **`aria-invalid` needs no mapping.** templUI's input is styled through
  `aria-invalid:` variants, the attribute the runtime sets on a refused
  field. Without JavaScript the re-rendered field is not marked
  ([#59](https://github.com/nkapatos/templatic/issues/59)), so it shows the
  message without the red border.
- **Each task's Done/Reopen button** is a submit button of its own
  `ui.FormFor(Toggle, "/toggle")` with `name="id"`.

## Without JavaScript

The page renders styled; the add form and the Done/Reopen buttons post
and redirect back, and a refused title renders the page again with 422
and the error. What degrades: the About button does nothing (the dialog
content is in the page but hidden), and the tabs stay on Open, so done
tasks cannot be seen. templUI offers no fallback for either.

## Content Security Policy

`@Scripts()`, `Head` and `Boot` all add the nonce from
`templ.WithNonce` to their script tags and module preloads, so the page
runs under `script-src 'self'` and under a nonce policy
(`script-src 'nonce-…' 'strict-dynamic'`) alike.

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the task store and its region, the command handlers, the mux |
| `components/tasks.go` | `Task`, the `Add` and `Toggle` commands and their input |
| `components/home.templ` | the page, the About dialog, the `TaskList` region markup |
| `components/layout.templ` | the document, `Head`, templUI's `Scripts`, `Boot` |
| `components/<name>/`, `components/baseui/`, `components/scripts*.{templ,go}`, `utils/` | templUI's components, copied by its CLI |
| `components.json` | templUI's CLI config: style, CSS file, import aliases, script output |
| `tailwind.css`, `tw-animate.css`, `shadcn-tailwind.css` | Tailwind's input with templUI's theme, and the stylesheets `init` vendored |
| `package.json` | the `css`, `generate` and `dev` scripts; Tailwind and concurrently |
| `client/app.ts` | the bundle entry: imports Tailwind's output |
| `public/favicon.svg` | a file the app serves itself; `public/js/` gets templUI's bundle |

`*_templ.go` files are generated by `templatic generate` and committed, as
with templ. `.templatic/`, `client/tailwind.out.css` and
`public/js/shadcn-templ-*.js` are generated and ignored.
