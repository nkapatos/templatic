// Package budget reports each entry's gzip size and fails the build when an entry exceeds its limit.
//
// For every JavaScript entry the bundle wrote, the report shows the gzip
// size of the entry together with its static imports, and the raw size. An
// entry is named as in the manifest: "boot", "app", "islands/user-card",
// "modules/x". A limit for a name the build does not have is an error, as is
// a limit that is not positive.
//
// With no limits the plugin only reports. The report goes to os.Stdout unless
// [Output] says otherwise.
//
// Usage in an app build program (see [github.com/nkapatos/templatic/cli]):
//
//	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr,
//		cli.With(budget.New(
//			budget.Limit("boot", 20),
//			budget.Limit("islands/big-chart", 60),
//		)),
//	))
//
// Or with the generate package directly:
//
//	generate.Run(generate.Options{Plugins: []generate.Plugin{budget.New(budget.Limit("boot", 20))}})
//
// The plugin hooks [generate.Builder.OnDone], so it runs after tc.json is
// written.
package budget

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/nkapatos/templatic/generate"
)

// Option configures the plugin.
type Option func(*plugin)

// Limit fails the build when entry's gzip size exceeds kb; an empty entry, a kb that is not positive, or an entry the build lacks is an error.
func Limit(entry string, kb float64) Option {
	return func(p *plugin) {
		if entry == "" || !(kb > 0) {
			p.bad = append(p.bad, fmt.Errorf("limit %q=%v: want an entry and a positive KB", entry, kb))
			return
		}
		p.limits[entry] = kb
	}
}

// Output sets where the report is written (default os.Stdout).
func Output(w io.Writer) Option { return func(p *plugin) { p.out = w } }

type plugin struct {
	limits map[string]float64
	bad    []error
	out    io.Writer
}

// New returns the plugin. With no limits it only reports.
func New(opts ...Option) generate.Plugin {
	p := &plugin{limits: map[string]float64{}, out: os.Stdout}
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *plugin) Name() string { return "budget" }

func (p *plugin) Setup(b *generate.Builder) error {
	if err := errors.Join(p.bad...); err != nil {
		return err
	}
	b.OnDone(p.done)
	return nil
}

var hashSuffix = regexp.MustCompile(`-[0-9a-f]{10}$`)

type row struct {
	name  string
	bytes int
	gz    int
}

func (p *plugin) done(r *generate.Result) error {
	byPath := map[string]generate.Output{}
	for _, o := range r.Outputs {
		byPath[o.Path] = o
	}
	distRel, err := filepath.Rel(r.Root, r.Dist)
	if err != nil {
		return err
	}
	distRel = filepath.ToSlash(distRel)

	var rows []row
	for _, o := range r.Outputs {
		if o.Entry == "" || filepath.Ext(o.Path) != ".js" {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(o.Path, distRel+"/"), ".js")
		name = hashSuffix.ReplaceAllString(name, "")
		seen := map[string]bool{}
		var walk func(path string)
		walk = func(path string) {
			if seen[path] {
				return
			}
			seen[path] = true
			for _, im := range byPath[path].Imports {
				if im.Kind == "import-statement" {
					walk(im.Path)
				}
			}
		}
		walk(o.Path)
		if css := strings.TrimSuffix(o.Path, ".js") + ".css"; byPath[css].Path != "" {
			seen[css] = true
		}
		row := row{name: name}
		for path := range seen {
			data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(path)))
			if err != nil {
				continue
			}
			row.bytes += len(data)
			g, err := gzipSize(data)
			if err != nil {
				return err
			}
			row.gz += g
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b row) int { return cmp.Compare(a.name, b.name) })

	known := map[string]bool{}
	var over []string
	fmt.Fprintf(p.out, "budget: %d entr%s (gzip, entry + static imports)\n", len(rows), plural(len(rows)))
	for _, rw := range rows {
		known[rw.name] = true
		limit, has := p.limits[rw.name]
		status := ""
		if has {
			status = fmt.Sprintf("  limit %.1f KB", limit)
			if float64(rw.gz) > limit*1024 {
				status += "  OVER"
				over = append(over, fmt.Sprintf("%s is %.1f KB gz, over its %.1f KB budget", rw.name, kb(rw.gz), limit))
			}
		}
		fmt.Fprintf(p.out, "  %-32s %8.1f KB gz  (%.1f KB raw)%s\n", rw.name, kb(rw.gz), kb(rw.bytes), status)
	}
	var unknown []string
	for name := range p.limits {
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	if len(unknown) > 0 {
		return fmt.Errorf("limit for unknown entr%s: %s", plural(len(unknown)), strings.Join(unknown, ", "))
	}
	if len(over) > 0 {
		return fmt.Errorf("over budget: %s", strings.Join(over, "; "))
	}
	return nil
}

func kb(n int) float64 { return float64(n) / 1024 }

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func gzipSize(data []byte) (int, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	if _, err := w.Write(data); err != nil {
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	return buf.Len(), nil
}
