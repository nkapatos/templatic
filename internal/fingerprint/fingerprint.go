package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Of returns the first 10 hex chars of the SHA-256 of root/rel.
func Of(root, rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("templatic: fingerprinting %s: %w", rel, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:10], nil
}

// Cache memoises file hashes by path, mtime and size.
type Cache struct {
	mu    sync.Mutex
	entry map[string]entry_
}

type entry_ struct {
	modUnixNano int64
	size        int64
	hash        string
}

func (c *Cache) Get(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entry == nil {
		c.entry = map[string]entry_{}
	}
	if e, ok := c.entry[path]; ok && e.modUnixNano == info.ModTime().UnixNano() && e.size == info.Size() {
		return e.hash, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])[:10]
	c.entry[path] = entry_{modUnixNano: info.ModTime().UnixNano(), size: info.Size(), hash: hash}
	return hash, nil
}
