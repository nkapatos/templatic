package generate

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/nkapatos/templatic/internal/buildfiles"
)

func bundle(opts Options, s *Summary, b *Builder) error {
	absRoot, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}
	if err := checkClientDeps(absRoot, opts, s.Islands); err != nil {
		return err
	}
	dist := filepath.Join(absRoot, filepath.FromSlash(opts.DistDir))
	if err := ensureDist(dist); err != nil {
		return err
	}

	// Explicit OutputPaths pin the layout: esbuild otherwise places outputs relative to
	// the entries' common ancestor, which moves boot.js once an island appears.
	table := &Entries{Root: absRoot, List: []Entry{{
		Name:  "boot",
		Input: filepath.Join(absRoot, GenDir, "runtime", "boot.ts"),
		Kind:  EntryBoot,
	}}}
	for _, isl := range s.Islands {
		table.List = append(table.List, Entry{
			Name:  "islands/" + isl.Tag,
			Input: filepath.Join(absRoot, opts.ClientDir, "islands", isl.Tag+".ts"),
			Kind:  EntryIsland,
		})
	}
	if app := filepath.Join(absRoot, opts.ClientDir, "app.ts"); fileExists(app) {
		table.List = append(table.List, Entry{Name: "app", Input: app, Kind: EntryExtra})
	}
	// ReadDir, not Glob: Glob reads '[' in an app's path as a pattern and matches nothing.
	modDir := filepath.Join(absRoot, opts.ClientDir, "modules")
	mods, err := filesWithSuffix(modDir, ".ts")
	if err != nil {
		return err
	}
	for _, m := range mods {
		if strings.HasSuffix(m, ".d.ts") {
			continue
		}
		table.List = append(table.List, Entry{Name: "modules/" + strings.TrimSuffix(m, ".ts"), Input: filepath.Join(modDir, m), Kind: EntryExtra})
	}
	if err := runHooks(b.entries, table, func(f func(*Entries) error, e *Entries) error { return f(e) }); err != nil {
		return err
	}
	if err := table.validate(); err != nil {
		return err
	}
	entries := make([]api.EntryPoint, 0, len(table.List))
	for _, en := range table.List {
		entries = append(entries, api.EntryPoint{InputPath: en.Input, OutputPath: en.Name})
	}

	buildOpts := api.BuildOptions{
		EntryPointsAdvanced: entries,
		Outdir:              dist,
		Bundle:              true,
		Splitting:           true,
		Format:              api.FormatESModule,
		Metafile:            true,
		Write:               true,
		AbsWorkingDir:       absRoot,
		ChunkNames:          "chunks/[name]-[hash]",
		AssetNames:          "assets/[name]-[hash]",
		// Absolute URLs: the default relative path breaks in <img src> and css.
		PublicPath:        strings.TrimSuffix(opts.AssetsBase, "/"),
		LogLevel:          api.LogLevelWarning,
		Target:            api.ES2022,
		MinifyWhitespace:  true,
		MinifySyntax:      true,
		MinifyIdentifiers: true,
		LegalComments:     api.LegalCommentsEndOfFile,
	}
	if opts.Mode == ModeDev {
		buildOpts.MinifyWhitespace = false
		buildOpts.MinifySyntax = false
		buildOpts.MinifyIdentifiers = false
		buildOpts.Sourcemap = api.SourceMapLinked
		buildOpts.Define = map[string]string{
			"__TC_DEV__":           "true",
			"process.env.NODE_ENV": `"development"`,
		}
	}
	if len(opts.Defines) > 0 {
		if buildOpts.Define == nil {
			buildOpts.Define = map[string]string{}
		}
		for k, v := range opts.Defines {
			buildOpts.Define[k] = v
		}
	}
	// esbuild's Alias also rewrites subpaths, so each subpath is aliased on its own.
	buildOpts.Alias = map[string]string{
		"templatic":          "./" + GenDir + "/runtime/index.ts",
		"templatic/commands": "./" + GenDir + "/commands.ts",
		"templatic/islands":  "./" + GenDir + "/islands",
	}
	for k, v := range opts.Alias {
		buildOpts.Alias[k] = v
	}
	buildOpts.Loader = map[string]api.Loader{}
	for ext, l := range defaultAssetLoaders {
		buildOpts.Loader[ext] = l
	}
	for ext, l := range opts.Loader {
		buildOpts.Loader[ext] = l
	}
	buildOpts.Plugins = append(buildOpts.Plugins, assetSuffixPlugin())
	buildOpts.Plugins = append(buildOpts.Plugins, opts.ESBuildPlugins...)
	buildOpts.Plugins = append(buildOpts.Plugins, b.esbuild...)
	if err := runHooks(b.buildOpts, &buildOpts, func(f func(*api.BuildOptions) error, o *api.BuildOptions) error { return f(o) }); err != nil {
		return err
	}
	if opts.Build != nil {
		opts.Build(&buildOpts)
	}

	result := api.Build(buildOpts)
	if len(result.Errors) > 0 {
		var msgs []string
		for _, e := range result.Errors {
			loc := ""
			if e.Location != nil {
				loc = fmt.Sprintf(" (%s:%d:%d)", e.Location.File, e.Location.Line, e.Location.Column)
				if e.Location.LineText != "" {
					loc += "\n    " + e.Location.LineText
				}
			}
			fmt.Fprintf(os.Stderr, "generate: esbuild: %s%s\n", e.Text, loc)
			msgs = append(msgs, e.Text+loc)
		}
		return fmt.Errorf("generate: bundle failed:\n%s", strings.Join(msgs, "\n"))
	}

	if buildOpts.Sourcemap == api.SourceMapLinked {
		if err := bareSourcemapLinks(absRoot, result.Metafile); err != nil {
			return err
		}
	}

	islandChunks, islandStyles, outputs, err := mapIslands(result.Metafile, opts.DistDir, s.Islands)
	if err != nil {
		return err
	}

	// Chunks are hashed by ChunkNames; boot.js and tc.json keep stable names.
	renamed := map[string]string{}
	if err := hashIslandEntries(dist, opts.DistDir, islandChunks, islandStyles, outputs, renamed); err != nil {
		return err
	}
	if err := hashExtraEntries(dist, opts.DistDir, absRoot, table, result.Metafile, outputs, renamed); err != nil {
		return err
	}
	// boot.js is hashed too so an inlined manifest names it; a stable shim stays for other pages.
	if err := hashBoot(dist, opts.DistDir, outputs, renamed); err != nil {
		return err
	}

	binds := map[string][]ManifestBind{}
	for _, isl := range s.Islands {
		if len(isl.Binds) == 0 {
			continue
		}
		entries := make([]ManifestBind, 0, len(isl.Binds))
		for _, b := range isl.Binds {
			entries = append(entries, ManifestBind{Topic: b.Topic, Field: b.Field})
		}
		binds[isl.Tag] = entries
	}
	manifest := &Manifest{}
	if len(binds) > 0 {
		manifest.Binds = binds
	}
	finalOutputs, err := finalizeOutputs(result.Metafile, renamed)
	if err != nil {
		return err
	}
	manifest.Root = absRoot
	manifest.Dist = dist
	manifest.Outputs = finalOutputs
	manifest.Version = 2
	manifest.Mode = string(ModeProd)
	if opts.Mode == ModeDev {
		manifest.Mode = string(ModeDev)
	}
	manifest.Base = opts.AssetsBase
	manifest.Modules = buildModules(table, absRoot, opts.DistDir, finalOutputs)
	manifest.Assets = buildAssets(absRoot, opts.DistDir, finalOutputs)
	if err := runHooks(b.manifest, manifest, func(f func(*Manifest) error, m *Manifest) error { return f(m) }); err != nil {
		return err
	}
	manifestJSON, err := json.Marshal(manifest, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dist, buildfiles.Manifest), append(manifestJSON, '\n')); err != nil {
		return err
	}
	s.Manifest = filepath.ToSlash(filepath.Join(opts.DistDir, buildfiles.Manifest))
	s.Bundle = slices.Sorted(maps.Keys(outputs))

	// esbuild never deletes; the previous generation stays for pages still open.
	written := []string{buildfiles.Manifest}
	for out := range outputs {
		// Metafile output keys are AbsWorkingDir-relative.
		written = append(written, distRel(opts.DistDir, out))
	}
	if err := recordGeneration(dist, written); err != nil {
		return err
	}
	return runHooks(b.done, &Result{
		Root:     absRoot,
		Dist:     dist,
		Summary:  s,
		Manifest: manifest,
		Metafile: result.Metafile,
		Outputs:  finalOutputs,
	}, func(f func(*Result) error, r *Result) error { return f(r) })
}

var sourcemapLink = regexp.MustCompile(`(sourceMappingURL=)\S*?([^/\s]+\.map)`)

// bareSourcemapLinks rewrites sourceMappingURL to the bare map name: PublicPath prefixes
// it, but the map sits beside its file.
func bareSourcemapLinks(absRoot, metafileJSON string) error {
	var raw struct {
		Outputs map[string]jsontext.Value `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(metafileJSON), &raw); err != nil {
		return err
	}
	for p := range raw.Outputs {
		if !strings.HasSuffix(p, ".js") {
			continue
		}
		file := filepath.Join(absRoot, filepath.FromSlash(p))
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		patched := sourcemapLink.ReplaceAll(data, []byte("${1}${2}"))
		if string(patched) == string(data) {
			continue
		}
		if err := os.WriteFile(file, patched, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func finalizeOutputs(metafileJSON string, renamed map[string]string) ([]Output, error) {
	var raw struct {
		Outputs map[string]struct {
			Bytes      int                 `json:"bytes"`
			EntryPoint string              `json:"entryPoint"`
			CSSBundle  string              `json:"cssBundle"`
			Inputs     map[string]struct{} `json:"inputs"`
			Imports    []struct {
				Path string `json:"path"`
				Kind string `json:"kind"`
			} `json:"imports"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(metafileJSON), &raw); err != nil {
		return nil, err
	}
	final := func(p string) string {
		p = filepath.ToSlash(p)
		if r, ok := renamed[p]; ok {
			return r
		}
		return p
	}
	outs := make([]Output, 0, len(raw.Outputs))
	for p, o := range raw.Outputs {
		out := Output{Path: final(p), Entry: filepath.ToSlash(o.EntryPoint), Bytes: o.Bytes}
		if o.CSSBundle != "" {
			out.CSS = final(o.CSSBundle)
		}
		for in := range o.Inputs {
			out.Inputs = append(out.Inputs, filepath.ToSlash(in))
		}
		slices.Sort(out.Inputs)
		for _, im := range o.Imports {
			out.Imports = append(out.Imports, Import{Path: final(im.Path), Kind: im.Kind})
		}
		outs = append(outs, out)
	}
	slices.SortFunc(outs, func(a, b Output) int { return cmp.Compare(a.Path, b.Path) })
	return outs, nil
}

func distRel(distDir, p string) string {
	return strings.TrimPrefix(filepath.ToSlash(p), filepath.ToSlash(distDir)+"/")
}

func buildModules(table *Entries, absRoot, distDir string, outs []Output) map[string]ManifestModule {
	byPath := map[string]*Output{}
	byEntry := map[string]*Output{}
	for i := range outs {
		o := &outs[i]
		byPath[o.Path] = o
		if o.Entry != "" {
			byEntry[o.Entry] = o
		}
	}
	mods := map[string]ManifestModule{}
	for _, en := range table.List {
		rel, err := filepath.Rel(absRoot, en.Input)
		if err != nil {
			continue
		}
		o := byEntry[filepath.ToSlash(rel)]
		if o == nil {
			continue
		}
		var m ManifestModule
		if strings.HasSuffix(o.Path, ".css") {
			m.CSS = []string{distRel(distDir, o.Path)}
		} else {
			m.File = distRel(distDir, o.Path)
			if o.CSS != "" {
				m.CSS = []string{distRel(distDir, o.CSS)}
			}
			seen := map[string]bool{o.Path: true}
			var walk func(*Output)
			walk = func(cur *Output) {
				for _, im := range cur.Imports {
					if im.Kind != "import-statement" || seen[im.Path] || byPath[im.Path] == nil {
						continue
					}
					seen[im.Path] = true
					m.Imports = append(m.Imports, distRel(distDir, im.Path))
					walk(byPath[im.Path])
				}
			}
			walk(o)
		}
		mods[en.Name] = m
	}
	if len(mods) == 0 {
		return nil
	}
	return mods
}

func buildAssets(absRoot, distDir string, outs []Output) map[string]string {
	prefix := filepath.ToSlash(distDir) + "/assets/"
	var assets map[string]string
	for _, o := range outs {
		if o.Entry != "" || !strings.HasPrefix(o.Path, prefix) || strings.HasSuffix(o.Path, ".map") {
			continue
		}
		for _, in := range o.Inputs {
			if ns, rest, ok := strings.Cut(in, ":"); ok && strings.HasPrefix(ns, assetNamespacePrefix) {
				in = rest
			}
			if filepath.IsAbs(in) {
				rel, err := filepath.Rel(absRoot, in)
				if err != nil || strings.HasPrefix(rel, "..") {
					continue
				}
				in = filepath.ToSlash(rel)
			}
			if assets == nil {
				assets = map[string]string{}
			}
			assets[in] = distRel(distDir, o.Path)
		}
	}
	return assets
}

func hashIslandEntries(dist, distDir string, chunks, styles map[string]string, outputs map[string]bool, renamed map[string]string) error {
	rename := func(rel string) (string, error) {
		return hashFile(dist, distDir, rel, outputs, renamed)
	}
	for tag, rel := range chunks {
		hashed, err := rename(rel)
		if err != nil {
			return err
		}
		chunks[tag] = hashed
	}
	for tag, rel := range styles {
		hashed, err := rename(rel)
		if err != nil {
			return err
		}
		styles[tag] = hashed
	}
	return nil
}

func hashFile(dist, distDir, rel string, outputs map[string]bool, renamed map[string]string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dist, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("generate: hashing %s: %w", rel, err)
	}
	sum := sha256.Sum256(data)
	ext := path.Ext(rel)
	hashed := strings.TrimSuffix(rel, ext) + "-" + hex.EncodeToString(sum[:])[:10] + ext
	if err := os.Rename(filepath.Join(dist, filepath.FromSlash(rel)), filepath.Join(dist, filepath.FromSlash(hashed))); err != nil {
		return "", fmt.Errorf("generate: hashing %s: %w", rel, err)
	}
	// A linked sourcemap follows its file; the hash is over the pre-rewrite content.
	if err := followSourcemap(dist, distDir, rel, hashed, data, outputs, renamed); err != nil {
		return "", err
	}
	// A distDir-relative key would let the pruner delete the renamed files.
	key := filepath.ToSlash(filepath.Join(distDir, filepath.FromSlash(rel)))
	hashedKey := filepath.ToSlash(filepath.Join(distDir, filepath.FromSlash(hashed)))
	delete(outputs, key)
	outputs[hashedKey] = true
	renamed[key] = hashedKey
	return hashed, nil
}

func followSourcemap(dist, distDir, rel, hashed string, data []byte, outputs map[string]bool, renamed map[string]string) error {
	oldMap := filepath.Join(dist, filepath.FromSlash(rel)+".map")
	if _, err := os.Stat(oldMap); err != nil {
		return nil
	}
	newMap := hashed + ".map"
	if err := os.Rename(oldMap, filepath.Join(dist, filepath.FromSlash(newMap))); err != nil {
		return fmt.Errorf("generate: hashing %s.map: %w", rel, err)
	}
	// With a PublicPath the comment carries a prefix; rewrite it bare.
	link := regexp.MustCompile(`sourceMappingURL=\S*?` + regexp.QuoteMeta(path.Base(rel)) + `\.map`)
	patched := link.ReplaceAllString(string(data), "sourceMappingURL="+path.Base(newMap))
	if err := os.WriteFile(filepath.Join(dist, filepath.FromSlash(hashed)), []byte(patched), 0o644); err != nil {
		return err
	}
	oldKey := filepath.ToSlash(filepath.Join(distDir, filepath.FromSlash(rel)+".map"))
	newKey := filepath.ToSlash(filepath.Join(distDir, filepath.FromSlash(newMap)))
	delete(outputs, oldKey)
	outputs[newKey] = true
	renamed[oldKey] = newKey
	return nil
}

func hashExtraEntries(dist, distDir, absRoot string, table *Entries, metafileJSON string, outputs map[string]bool, renamed map[string]string) error {
	var raw struct {
		Outputs map[string]struct {
			EntryPoint string `json:"entryPoint"`
		} `json:"outputs"`
	}
	for _, en := range table.List {
		if en.Kind != EntryExtra {
			continue
		}
		if raw.Outputs == nil {
			if err := json.Unmarshal([]byte(metafileJSON), &raw); err != nil {
				return err
			}
		}
		relIn, err := filepath.Rel(absRoot, en.Input)
		if err != nil {
			return err
		}
		relIn = filepath.ToSlash(relIn)
		var jsOut string
		for outPath, o := range raw.Outputs {
			if o.EntryPoint != "" && filepath.ToSlash(o.EntryPoint) == relIn {
				jsOut = strings.TrimPrefix(filepath.ToSlash(outPath), filepath.ToSlash(distDir)+"/")
			}
		}
		if jsOut == "" {
			return fmt.Errorf("generate: entry %s produced no output", en.Name)
		}
		if _, err := hashFile(dist, distDir, jsOut, outputs, renamed); err != nil {
			return err
		}
		cssRel := en.Name + ".css"
		if _, err := os.Stat(filepath.Join(dist, filepath.FromSlash(cssRel))); err == nil {
			if _, err := hashFile(dist, distDir, cssRel, outputs, renamed); err != nil {
				return err
			}
		}
	}
	return nil
}

func hashBoot(dist, distDir string, outputs map[string]bool, renamed map[string]string) error {
	src := filepath.Join(dist, "boot.js")
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("generate: hashing boot: %w", err)
	}
	sum := sha256.Sum256(data)
	hashed := "boot-" + hex.EncodeToString(sum[:])[:10] + ".js"
	if err := os.Rename(src, filepath.Join(dist, hashed)); err != nil {
		return fmt.Errorf("generate: hashing boot: %w", err)
	}
	if err := followSourcemap(dist, distDir, "boot.js", hashed, data, outputs, renamed); err != nil {
		return err
	}
	oldKey := filepath.ToSlash(filepath.Join(distDir, "boot.js"))
	newKey := filepath.ToSlash(filepath.Join(distDir, hashed))
	delete(outputs, oldKey)
	outputs[newKey] = true
	renamed[oldKey] = newKey
	// Stable shim for apps without LoadManifest and cached pages that reference boot.js.
	if err := os.WriteFile(src, fmt.Appendf(nil, "import %s;\n", tsString("./"+hashed)), 0o644); err != nil {
		return fmt.Errorf("generate: writing boot shim: %w", err)
	}
	outputs[oldKey] = true
	return nil
}

func mapIslands(metafileJSON, distDir string, islands []Island) (map[string]string, map[string]string, map[string]bool, error) {
	var raw struct {
		Outputs map[string]struct {
			EntryPoint string `json:"entryPoint"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(metafileJSON), &raw); err != nil {
		return nil, nil, nil, err
	}
	if len(raw.Outputs) == 0 {
		return nil, nil, nil, fmt.Errorf("generate: metafile has no outputs")
	}

	chunks := map[string]string{}
	styles := map[string]string{}
	outputs := map[string]bool{}
	for outPath, out := range raw.Outputs {
		outputs[outPath] = true
		rel := strings.TrimPrefix(filepath.ToSlash(outPath), filepath.ToSlash(distDir)+"/")
		for _, isl := range islands {
			entry := filepath.ToSlash(out.EntryPoint)
			if entry != "" && strings.HasSuffix(entry, "islands/"+isl.Tag+".ts") {
				chunks[isl.Tag] = rel
				continue
			}
			// css has no entryPoint in the metafile; the layout pins it to islands/{tag}.css.
			if rel == "islands/"+isl.Tag+".css" {
				styles[isl.Tag] = rel
			}
		}
	}
	for _, isl := range islands {
		if _, ok := chunks[isl.Tag]; !ok {
			return nil, nil, nil, fmt.Errorf("generate: island %s produced no chunk", isl.Tag)
		}
	}
	return chunks, styles, outputs, nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
