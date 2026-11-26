// Package cli is the templatic command line: generate, dev, init, clean and check-ts.
//
// The templatic binary ([github.com/nkapatos/templatic/cmd/templatic]) is a
// one-line program around [Main]. Use Main yourself when an app needs plugins
// or code-set options (see "App build program" below).
//
// # Commands
//
// Every command takes the global flag --root (module root, default ".").
//
// generate renders Go glue for .templ files, generates island TypeScript,
// copies the client runtime and bundles. See [github.com/nkapatos/templatic/generate]
// for the steps. Flags:
//
//	--client dir        the app's client code (default "client")
//	--dist dir          bundle output directory (default ".templatic/dist")
//	--skip-bundle       skip the esbuild step
//	--mode prod|dev     build mode (default "prod")
//	--define KEY=VALUE  esbuild define, repeatable
//	--alias FROM=TO     esbuild import alias, repeatable
//	--loader .ext=kind  loader (file, dataurl or text) for an extension, repeatable
//
// dev takes the same flags, with --mode defaulting to "dev". It reruns generate
// when files change. Two more flags:
//
//	--exec command      run this shell command as the app and restart it
//	                    when .go files change, for example "go run ."
//	--debounce d        quiet time after the last change before a rebuild
//	                    (default 100ms)
//
// init scaffolds a new app into a directory: templatic init [options] dir.
//
//	--module path       Go module path (default: the directory name)
//	--version v         templatic version to require (default: this binary's)
//	--replace dir       path to a local templatic checkout (adds a go.mod replace)
//	--force             write into a non-empty directory; existing files are kept
//
// clean removes .templatic/, but only if templatic made it (it checks for the
// marker file).
//
// check-ts runs tsc --noEmit over the client, using .templatic/tsconfig.json or
// the app's own tsconfig.json. It needs a prior generate.
//
// # Exit codes
//
// Main returns 0 on success, 2 for a usage error (unknown command or flag,
// or no command) and 1 for any other error.
//
// # App build program
//
// Plugins are Go values, so the stock binary cannot load them. An app that uses
// plugins gets its own small main package that runs the same command line with
// the plugins registered, and calls that program instead of templatic:
//
//	package main
//
//	import (
//		"os"
//
//		"github.com/nkapatos/templatic/cli"
//		"github.com/nkapatos/templatic/generate"
//		"github.com/nkapatos/templatic/plugins/budget"
//		"github.com/nkapatos/templatic/plugins/sri"
//	)
//
//	func main() {
//		os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr,
//			cli.With(sri.New(), budget.New(budget.Limit("boot", 20))),
//			cli.Options(func(o *generate.Options) { o.ClientDir = "web" }),
//		))
//	}
//
// [With] adds plugins to generate and dev, after the flags are applied.
// [Options] changes the [generate.Options] those commands build from their
// flags, after the flags, so code has the last word.
package cli
