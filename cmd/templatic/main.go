// Command templatic is the templatic CLI.
package main

import (
	"os"

	"github.com/nkapatos/templatic/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
