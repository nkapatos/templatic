// Command templatic is the templatic command line.
//
// It builds Go glue, island TypeScript and the browser bundle for an app, and
// reruns them while you develop. The commands and flags are documented in
// package [github.com/nkapatos/templatic/cli].
//
// Install it:
//
//	go install github.com/nkapatos/templatic/cmd/templatic@latest
//
// Usage:
//
//	templatic init [--module path] [--version v] [--replace dir] [--force] dir
//	templatic generate [--mode prod|dev] [--client dir] [--dist dir] [--skip-bundle]
//	templatic dev [--exec command] [--debounce duration]
//	templatic clean
//	templatic check-ts
//
// Every command takes --root to name the module root (default ".").
// "templatic help" and "templatic command --help" list all flags.
//
// This binary has no plugins. An app that uses plugins builds its own binary
// around [github.com/nkapatos/templatic/cli.Main].
package main

import (
	"os"

	"github.com/nkapatos/templatic/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
