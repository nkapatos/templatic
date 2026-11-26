package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

var defaultAssetLoaders = map[string]api.Loader{
	".svg":   api.LoaderFile,
	".png":   api.LoaderFile,
	".jpg":   api.LoaderFile,
	".jpeg":  api.LoaderFile,
	".gif":   api.LoaderFile,
	".webp":  api.LoaderFile,
	".avif":  api.LoaderFile,
	".woff":  api.LoaderFile,
	".woff2": api.LoaderFile,
	".ttf":   api.LoaderFile,
}

// One namespace per suffix keeps two imports of the same file as two modules.
const assetNamespacePrefix = "tc-asset-"

func assetSuffixPlugin() api.Plugin {
	return api.Plugin{
		Name: "templatic-asset-suffix",
		Setup: func(b api.PluginBuild) {
			b.OnResolve(api.OnResolveOptions{Filter: `\?(url|raw|inline)$`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				i := strings.LastIndex(args.Path, "?")
				spec, kind := args.Path[:i], args.Path[i+1:]
				r := b.Resolve(spec, api.ResolveOptions{
					Kind:       args.Kind,
					ResolveDir: args.ResolveDir,
					Importer:   args.Importer,
					Namespace:  args.Namespace,
					PluginData: args.PluginData,
				})
				if len(r.Errors) > 0 {
					return api.OnResolveResult{Errors: r.Errors}, nil
				}
				if r.External {
					return api.OnResolveResult{Path: args.Path, External: true}, nil
				}
				return api.OnResolveResult{Path: r.Path, Namespace: assetNamespacePrefix + kind}, nil
			})
			for kind, loader := range map[string]api.Loader{
				"url":    api.LoaderFile,
				"raw":    api.LoaderText,
				"inline": api.LoaderDataURL,
			} {
				b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: assetNamespacePrefix + kind}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					data, err := os.ReadFile(args.Path)
					if err != nil {
						return api.OnLoadResult{}, fmt.Errorf("?%s: %w", kind, err)
					}
					contents := string(data)
					return api.OnLoadResult{
						Contents:   &contents,
						Loader:     loader,
						ResolveDir: filepath.Dir(args.Path),
						WatchFiles: []string{args.Path},
					}, nil
				})
			}
		},
	}
}
