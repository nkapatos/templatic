// Package clientruntime embeds templatic's browser runtime TypeScript sources.
package clientruntime

import (
	"embed"
	"encoding/json/v2"
	"io/fs"
)

//go:embed ts/*.ts package.json
var sources embed.FS

// Files returns the runtime's TypeScript sources keyed by base file name.
func Files() (map[string][]byte, error) {
	names, err := fs.Glob(sources, "ts/*.ts")
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(names))
	for _, n := range names {
		b, err := sources.ReadFile(n)
		if err != nil {
			return nil, err
		}
		out[n[len("ts/"):]] = b
	}
	return out, nil
}

// PeerDependencies returns the package.json peerDependencies: npm packages the app installs, with supported ranges.
func PeerDependencies() map[string]string {
	return packageJSON().Peer
}

// TestedVersions returns the exact devDependencies versions for the PeerDependencies packages.
func TestedVersions() map[string]string {
	p := packageJSON()
	out := make(map[string]string, len(p.Peer))
	for name := range p.Peer {
		v, ok := p.Dev[name]
		if !ok {
			panic("clientruntime: package.json: peer " + name + " has no devDependency")
		}
		out[name] = v
	}
	return out
}

type packageManifest struct {
	Peer map[string]string `json:"peerDependencies"`
	Dev  map[string]string `json:"devDependencies"`
}

func packageJSON() packageManifest {
	b, err := sources.ReadFile("package.json")
	if err != nil {
		panic("clientruntime: package.json not embedded: " + err.Error())
	}
	var p packageManifest
	if err := json.Unmarshal(b, &p); err != nil {
		panic("clientruntime: package.json: " + err.Error())
	}
	return p
}
