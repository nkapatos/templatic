package ui

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/markup"
)

// Module requests the client page module client/modules/{name}.ts; it loads
// once per page.
func Module(name string) templ.Component {
	if !markup.ValidName(name) {
		panic("templatic: module name must match [A-Za-z0-9_.-]+: " + name)
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := fmt.Fprintf(w, `<template data-tc-module=%q></template>`, name)
		return err
	})
}
