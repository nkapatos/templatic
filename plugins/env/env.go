// Package env exposes environment variables with an explicit prefix to client code as esbuild defines.
//
// Each variable whose name starts with the prefix becomes a define
// "process.env.NAME" whose value is the variable's value as a string
// literal, so client code can read process.env.TC_PUBLIC_API at build time.
// The prefix is required: an empty prefix is an error, so the whole
// environment is never exposed. A variable that equals the prefix exactly is
// skipped. Defines set in [generate.Options.Defines] win over these.
//
// Usage in an app build program (see [github.com/nkapatos/templatic/cli]):
//
//	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr,
//		cli.With(env.New("TC_PUBLIC_")),
//	))
//
// [Lookup] replaces os.Environ as the source, which tests use.
// The plugin hooks [generate.Builder.OnConfig].
package env

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/nkapatos/templatic/generate"
)

// Option configures the plugin.
type Option func(*plugin)

// Lookup replaces os.Environ as the source; entries are "KEY=VALUE".
func Lookup(environ func() []string) Option { return func(p *plugin) { p.environ = environ } }

type plugin struct {
	prefix  string
	environ func() []string
}

// New returns the plugin exposing variables that start with prefix.
func New(prefix string, opts ...Option) generate.Plugin {
	p := &plugin{prefix: prefix, environ: os.Environ}
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *plugin) Name() string { return "env" }

func (p *plugin) Setup(b *generate.Builder) error {
	if p.prefix == "" {
		return errors.New("a non-empty prefix is required (the whole environment is never exposed)")
	}
	b.OnConfig(func(o *generate.Options) error {
		defs := p.defines()
		if len(defs) == 0 {
			return nil
		}
		merged := make(map[string]string, len(defs)+len(o.Defines))
		for k, v := range defs {
			merged[k] = v
		}
		for k, v := range o.Defines {
			merged[k] = v
		}
		o.Defines = merged
		return nil
	})
	return nil
}

func (p *plugin) defines() map[string]string {
	out := map[string]string{}
	env := p.environ()
	slices.Sort(env)
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(k, p.prefix) || len(k) == len(p.prefix) {
			continue
		}
		q, _ := json.Marshal(v, jsontext.AllowInvalidUTF8(true))
		out["process.env."+k] = string(q)
		out["import.meta.env."+k] = string(q)
	}
	return out
}
