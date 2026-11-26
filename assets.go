package templatic

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nkapatos/templatic/internal/fingerprint"
)

// Assets serves dir with the cache policy the generated layout expects: hashed
// files immutable, fixed names no-cache, directories 404, errors no-store.
func Assets(dir string) http.Handler {
	fileServer := http.FileServer(filesOnly{http.Dir(dir)})
	fingerprints := &fingerprint.Cache{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		switch {
		case isContentAddressed(clean):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case versionMatches(dir, clean, r.URL.Query().Get("v"), fingerprints):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(noStoreOnError{w}, r)
	})
}

// filesOnly hides directories from http.FileServer.
type filesOnly struct{ fs http.FileSystem }

func (f filesOnly) Open(name string) (http.File, error) {
	file, err := f.fs.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		_ = file.Close()
		return nil, fs.ErrNotExist
	}
	return file, nil
}

// noStoreOnError sets no-store on error responses.
type noStoreOnError struct{ http.ResponseWriter }

func (w noStoreOnError) WriteHeader(code int) {
	if code >= 400 {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.ResponseWriter.WriteHeader(code)
}

// ReadFrom keeps the sendfile path http.FileServer reaches through io.Copy.
func (w noStoreOnError) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(w.ResponseWriter, r)
}

func (w noStoreOnError) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// versionMatches reports whether v is the current fingerprint of the file at
// urlPath.
func versionMatches(root, urlPath, v string, cache *fingerprint.Cache) bool {
	if v == "" {
		return false
	}
	hash, err := cache.Get(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(urlPath, "/"))))
	if err != nil {
		return false
	}
	return hash == v
}

// contentHashSuffix matches content-hashed names such as app-{10 hex}.js.
var contentHashSuffix = regexp.MustCompile(`-[0-9a-f]{10}\.(js|css)$`)

// bundledAsset matches hashed bundler assets, assets/{name}-{hash}.ext.
var bundledAsset = regexp.MustCompile(`(^|/)assets/[^/]+-[A-Z0-9]{8}\.[A-Za-z0-9]+$`)

func isContentAddressed(p string) bool {
	if contentHashSuffix.MatchString(p) || bundledAsset.MatchString(p) {
		return true
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "chunks", "islands":
			return true
		}
	}
	return false
}

// AssetFingerprint returns the 10-hex content hash of rel under root, for ?v= on
// files served by Assets.
func AssetFingerprint(root, rel string) (string, error) { return fingerprint.Of(root, rel) }
