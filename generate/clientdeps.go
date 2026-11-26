package generate

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/nkapatos/templatic/internal/clientruntime"
)

func checkClientDeps(absRoot string, opts Options, islands []Island) error {
	peers := clientruntime.PeerDependencies()
	tested := clientruntime.TestedVersions()
	need := []string{"idiomorph"}
	for _, isl := range islands {
		if isl.Variant == "lit" {
			need = append(need, "lit")
			break
		}
	}
	var problems, install []string
	for _, pkg := range need {
		if _, ok := opts.Alias[pkg]; ok {
			continue
		}
		rng, want := peers[pkg], tested[pkg]
		got, where := installedVersion(absRoot, pkg)
		if got == "" {
			problems = append(problems, fmt.Sprintf("%s is not installed (templatic needs %s, tested with %s)", pkg, rng, want))
			install = append(install, pkg+"@"+want)
			continue
		}
		ok, err := satisfies(rng, got)
		if err != nil && errors.Is(err, errUnsupportedRange) {
			return fmt.Errorf("generate: %s: %w", pkg, err)
		}
		if ok {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s %s is installed (%s), templatic needs %s (tested with %s)", pkg, got, where, rng, want))
		install = append(install, pkg+"@"+want)
	}
	if len(problems) == 0 {
		return nil
	}
	slices.Sort(install)
	return fmt.Errorf("generate: the client bundle needs npm packages the app must install:\n  %s\n  run: npm install --save-exact %s",
		strings.Join(problems, "\n  "), strings.Join(install, " "))
}

var errUnsupportedRange = errors.New("unsupported version range in templatic's package.json (templatic bug; supported: x.y.z, ^x.y.z, ~x.y.z)")

func satisfies(rng, version string) (bool, error) {
	op, base := "", rng
	if strings.HasPrefix(rng, "^") || strings.HasPrefix(rng, "~") {
		op, base = rng[:1], rng[1:]
	}
	if !isPlainVersion(base) {
		return false, fmt.Errorf("%w: %q", errUnsupportedRange, rng)
	}
	lo := "v" + base
	v := "v" + version
	if !semver.IsValid(v) {
		return false, nil
	}
	v = strings.TrimSuffix(v, semver.Build(v))
	if semver.Prerelease(v) != "" {
		return false, nil
	}
	parts := strings.Split(base, ".")
	major, minor, patch := parts[0], parts[1], parts[2]
	var hi string
	switch op {
	case "":
		return semver.Compare(v, lo) == 0, nil
	case "~":
		hi = major + "." + incr(minor) + ".0"
	case "^":
		switch {
		case major != "0":
			hi = incr(major) + ".0.0"
		case minor != "0":
			hi = "0." + incr(minor) + ".0"
		default:
			hi = "0.0." + incr(patch)
		}
	}
	if semver.Compare(v, lo) < 0 {
		return false, nil
	}
	return semver.Compare(v, "v"+hi) < 0, nil
}

func isPlainVersion(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func incr(n string) string {
	i, err := strconv.Atoi(n)
	if err != nil {
		panic("generate: incr: " + err.Error())
	}
	return strconv.Itoa(i + 1)
}

func installedVersion(root, pkg string) (version, path string) {
	for dir := root; ; dir = filepath.Dir(dir) {
		p := filepath.Join(dir, "node_modules", filepath.FromSlash(pkg), "package.json")
		if b, err := os.ReadFile(p); err == nil {
			var m struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(b, &m) == nil && m.Version != "" {
				return m.Version, p
			}
		}
		if parent := filepath.Dir(dir); parent == dir {
			return "", ""
		}
	}
}
