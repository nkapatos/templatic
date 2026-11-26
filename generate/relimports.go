package generate

import (
	"path/filepath"
	"regexp"
	"strings"
)

// The SFC script and style are written to GenDir/islands but authored against ClientDir/islands, so relative references are rebased.
var (
	jsRelImport  = regexp.MustCompile(`(\bfrom\s*|\bimport\s*\(?\s*)(["'])(\.{1,2}/[^"'\n]*)(["'])`)
	cssRelImport = regexp.MustCompile(`(@import\s+(?:url\(\s*)?|url\(\s*)(["']?)(\.{1,2}/[^"')\s]*)(["']?)`)
)

func rebaseRelative(opts Options, src string, re *regexp.Regexp) string {
	from := filepath.Join(GenDir, "islands")
	to := filepath.Join(opts.ClientDir, "islands")
	return re.ReplaceAllStringFunc(src, func(m string) string {
		g := re.FindStringSubmatch(m)
		target := filepath.Join(to, filepath.FromSlash(g[3]))
		rel, err := filepath.Rel(from, target)
		if err != nil {
			return m
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "../") {
			rel = "./" + rel
		}
		return g[1] + g[2] + rel + g[4]
	})
}
