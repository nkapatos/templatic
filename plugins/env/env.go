// Package env exposes environment variables with an explicit prefix to client code as esbuild defines;
// an empty prefix is an error.
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
