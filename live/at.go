package live

import "strings"

// At declares that a successful command leaves the session at url; url must be a
// same-app path starting with "/", else it panics.
func At(url string) Push {
	if !strings.HasPrefix(url, "/") || strings.HasPrefix(url, "//") || strings.Contains(url, "://") {
		panic("live: At URL must be same-app relative (a leading /, no scheme or host): " + url)
	}
	return Push{url: url}
}

// Push is the receipt At returns: the command succeeded and the session is at a
// URL.
type Push struct{ url string }

// URL returns the declared address.
func (p Push) URL() string { return p.url }
