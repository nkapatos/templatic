package ui

import (
	"encoding/json/v2"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/markup"
)

// Hook returns attributes marking an element for a client hook, with args
// JSON-encoded. It panics on an invalid name or unencodable args.
func Hook(name string, args any) templ.Attributes {
	if !markup.ValidName(name) {
		panic("templatic: hook name must match [A-Za-z0-9_.-]+: " + name)
	}
	attrs := templ.Attributes{"data-tc-hook": name}
	if args != nil {
		enc, err := json.Marshal(args)
		if err != nil {
			panic("templatic: hook " + name + " args: " + err.Error())
		}
		attrs["data-tc-hook-args"] = string(enc)
	}
	return attrs
}
