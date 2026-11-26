// Package generate is templatic's build step: it turns .templ files and client code into Go glue, TypeScript and a browser bundle.
//
// [Run] is the whole API for a one-off build. The templatic command and
// [github.com/nkapatos/templatic/cli] call it for "templatic generate" and for
// every rebuild of "templatic dev". Call it directly from a test or from your
// own build program when you need control the command line does not give.
//
// # What Run does
//
// Run works under [Options.Root] and stops at the first error. Outputs that
// were already written stay in place. The steps are:
//
//  1. Apply defaults, run each plugin's Setup and its [Builder.OnConfig]
//     hooks, then check the paths. Create [GenDir] and the dist directory,
//     refusing a directory that exists but was not made by templatic.
//  2. Find the .templ files under the root. The walk skips node_modules,
//     vendor, .git, dist, tc_gen and .templatic. Delete x_templ.go and
//     x_tc.go files whose x.templ is gone; a hand-written file with the
//     same name is left alone.
//  3. For each .templ file, read the //templatic:island and
//     //templatic:region pragmas, take out the <script> and <style> of island
//     components, and write x_templ.go (the templ output) and x_tc.go (the Go
//     glue: an island constructor or a session region declaration per
//     pragma). Both sit next to the .templ file. Check that every package
//     qualifier the glue uses is imported or declared.
//  4. Type-check the module's packages with golang.org/x/tools/go/packages.
//     Check that each island bind names a cell topic that exists.
//  5. Write .templatic/commands.ts from the live.NewCommand calls found in
//     the program, so client code can call commands with their Go types.
//  6. For each island, write the props types, hooks and base modules and the
//     extracted CSS into .templatic/islands. Create the island's define module,
//     client/islands/{tag}.ts, only if it does not exist yet; it is yours to
//     edit after that.
//  7. Copy the client runtime into .templatic/runtime and write
//     .templatic/tsconfig.json.
//  8. Run the [Builder.OnScan] hooks. With [Options.SkipBundle] the run ends
//     here.
//  9. Bundle with esbuild into the dist directory. The entries are the
//     runtime boot module, one per island, client/app.ts if present, and each
//     client/modules/*.ts. [Builder.OnEntries] hooks may add more. Outputs
//     get content hashes in their names.
//  10. Write the manifest tc.json into the dist directory, after the
//     [Builder.OnManifest] hooks, then run the [Builder.OnDone] hooks.
//
// # What it writes
//
// Everything except the Go glue and the island define modules lives in
// [GenDir], ".templatic" under the root. It holds a marker file, the copied
// runtime, commands.ts, tsconfig.json, the generated island modules and, by
// default, the bundle in .templatic/dist ([DefaultDistDir]). The directory is
// meant to be gitignored and is safe to delete ("templatic clean" does it).
// Serve the dist directory under [DefaultAssetsBase], or whatever
// [Options.AssetsBase] says, and let the templatic package read tc.json.
//
// # Island pragma
//
// A component becomes an island when a comment line directly above it in the
// .templ file reads
//
//	//templatic:island tag=user-card props=CardProps
//
// The fields are space separated key=value pairs:
//
//   - tag (required): the custom element name, lowercase kebab with at least
//     one dash, starting with a letter.
//   - props (required): the Go struct that holds the island's props. Its
//     json/v2 field names become the TypeScript props interface.
//   - variant: "hooks" (the default) or "lit". A hooks island keeps its
//     server-rendered markup and is wired with onMount; a lit island is a
//     LitElement subclass.
//   - bind, repeatable, lit only: field:topic pushes the value of the cell
//     named topic into the props field. The field is the props struct's json
//     name; the topic uses letters, digits, underscore, dash and dot.
//
// For example, a lit island with two bound props:
//
//	//templatic:island tag=big-chart props=ChartProps variant=lit bind=focus:chart.focus bind=range:chart.range
//
// A component carries at most one pragma. Unknown fields, a missing =value
// or a bad tag are errors that name the pragma line.
//
// A second pragma, //templatic:region topic=name (or topic-of=Const, naming a
// Go constant), declares a session region rendered by the component below
// it, as live.DeclareSessionRegion does.
//
// # Plugins
//
// A [Plugin] has a name and a Setup method that receives a [Builder]. Setup
// registers hooks; a Setup or hook error aborts the run and is reported with
// the plugin's name. Hooks run in plugin registration order, at these points:
//
//   - [Builder.OnConfig]: adjust [Options] before anything runs.
//   - [Builder.OnScan]: after the glue, TypeScript and runtime are written,
//     with the [Summary] so far.
//   - [Builder.OnEntries]: add or alter esbuild entries ([Entries.Add]).
//   - [Builder.ESBuild]: add a native esbuild plugin; [ESBuild] wraps one as
//     a [Plugin].
//   - [Builder.OnBuildOptions]: change the esbuild options last, before
//     [Options.Build].
//   - [Builder.OnManifest]: edit the [Manifest] before tc.json is written.
//   - [Builder.OnDone]: receive the [Result], everything the bundle produced.
//   - [Builder.Watch]: ask the dev loop to watch extra root-relative globs;
//     the globs come back in [Summary.Watch].
//
// This is a minimal plugin that records the build mode in tc.json:
//
//	type modePlugin struct{}
//
//	func (modePlugin) Name() string { return "mode-note" }
//
//	func (modePlugin) Setup(b *generate.Builder) error {
//		b.OnManifest(func(m *generate.Manifest) error {
//			if m.Meta == nil {
//				m.Meta = map[string]any{}
//			}
//			m.Meta["mode-note"] = m.Mode
//			return nil
//		})
//		return nil
//	}
//
// Pass it in [Options.Plugins], or give it to [github.com/nkapatos/templatic/cli.With]
// in an app build program. The first-party plugins live in
// [github.com/nkapatos/templatic/plugins/budget],
// [github.com/nkapatos/templatic/plugins/env] and
// [github.com/nkapatos/templatic/plugins/sri].
//
// # Usage
//
//	sum, err := generate.Run(generate.Options{
//		Root: ".",
//		Mode: generate.ModeDev,
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(len(sum.Islands), "islands,", len(sum.Bundle), "bundle files")
//
// [Summary] lists the Go files emitted, the islands found, the runtime files
// copied, the bundle outputs and the manifest path.
package generate
