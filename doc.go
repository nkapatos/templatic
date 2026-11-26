// Package templatic provides server-rendered, reactive UIs built on templ.
//
// A templatic app is plain net/http. The app owns its main function, its
// [net/http.ServeMux] and its pages; this package supplies the handlers and
// middleware the app mounts, and the templ components a layout places.
// There is no templatic server type. See the repository README for the
// general overview and the templatic command for project setup and builds.
//
// Three packages work together:
//
//   - This package serves the generated client bundle and lets pages find it:
//     [Assets], the manifest ([ManifestFor], [LoadManifest]), the middleware
//     ([App], [WithAssets], [WithPageTokens]) and the layout components
//     [Head] and [Boot].
//   - [github.com/nkapatos/templatic/live] holds the server-side state: a
//     [live.World] with cells, regions, streams and commands, pushed to
//     browsers over Server-Sent Events.
//   - [github.com/nkapatos/templatic/ui] holds the templ components that
//     bind markup to that state (ui.Cell, ui.Region, ui.Button and others).
//
// # Mounting an app
//
// The order below is the one the generated project's main.go follows.
//
//  1. Create the world with [live.NewWorld], passing [live.WithKey] in
//     production so page tokens survive restarts, and declare its cells,
//     regions and commands.
//  2. Load the manifest that "templatic generate" wrote into the dist
//     directory: [ManifestFor] with the mode ("dev" follows rebuilds, any
//     other value reads it once). Call Notify on the result with the world
//     so that dev rebuilds reload open pages.
//  3. Register the app's pages and no-JS form routes on a mux.
//  4. Mount the world with [live.World.Mount], which adds GET base/stream and
//     POST base/commands.
//  5. Serve the bundle with [Assets] on the path the layout names as its
//     Assets base (conventionally /_templatic/), behind
//     [net/http.StripPrefix].
//  6. Wrap the mux in [App] and pass the result to [net/http.ListenAndServe].
//
// A condensed version:
//
//	w := live.NewWorld(live.WithKey(key))
//	count := live.NewCell(w, "count", 0)
//
//	manifests, err := templatic.ManifestFor(os.Getenv("TC_MODE"), ".templatic/dist")
//	if err != nil {
//		log.Fatal(err)
//	}
//	manifests.Notify(w)
//
//	mux := http.NewServeMux()
//	mux.Handle("GET /{$}", templ.Handler(Home(count)))
//	w.Mount(mux, "/live")
//	mux.Handle("GET /_templatic/", http.StripPrefix("/_templatic/",
//		templatic.Assets(".templatic/dist")))
//
//	log.Fatal(http.ListenAndServe(":8080", templatic.App(w, manifests, mux)))
//
// # Layouts
//
// A layout places [Head] in the document head and [Boot] once at the end of
// the body, both with the same Assets base. Boot emits the live
// configuration (the stream and command URLs, the page token) and the boot
// module script. Place any [github.com/nkapatos/templatic/ui.Island]
// before Boot so that Boot can preload its module.
//
//	@templatic.Head(templatic.HeadOptions{Assets: "/_templatic"})
//	...
//	@templatic.Boot(templatic.BootOptions{Live: "/live", Assets: "/_templatic"})
//
// # Middleware and page tokens
//
// [WithAssets] stores the manifest in the request context, so Head and Boot
// need no manifest argument. [WithPageTokens] lets a page obtain the signed
// token that ties a rendered page to one live session. Boot reads it with
// [PageToken], which mints the token on first use. A page that wants its
// session to start with preset cell values or attested parameters calls
// [SeedPage] with [live.SeedCell] or [live.TokenParam] before the token is
// minted; calling it afterwards panics. [App] applies both middleware
// around a mux and skips a layer whose argument is nil.
//
// # Assets and the manifest
//
// [Assets] serves the generated directory with its cache policy: content
// hashed files are immutable, fixed names such as boot.js revalidate, and
// directories are not listed. [Asset] returns the hashed URL of a bundled
// file by source path, after [UseManifest] registered a manifest.
// [AssetFingerprint] returns the content hash of a file, for a ?v= query on
// files that [Assets] serves.
package templatic
