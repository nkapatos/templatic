package live

import "strings"

// At declares that a successful command leaves the session at url, a new history
// entry; url must be a same-app path starting with "/", else it panics.
func At(url string) Push {
	checkAppURL(url)
	return Push{url: url}
}

// Replace is At that replaces the current history entry instead of adding one, as
// search-as-you-type or a filter wants.
func Replace(url string) Push {
	checkAppURL(url)
	return Push{url: url, replace: true}
}

func checkAppURL(url string) {
	if !strings.HasPrefix(url, "/") || strings.HasPrefix(url, "//") || strings.Contains(url, "://") {
		panic("live: At URL must be same-app relative (a leading /, no scheme or host): " + url)
	}
}

// Push is the receipt At and Replace return: the command succeeded and the session
// is at a URL.
type Push struct {
	url     string
	replace bool
}

// URL returns the declared address.
func (p Push) URL() string { return p.url }

// Replaces reports whether the address replaces the current history entry.
func (p Push) Replaces() bool { return p.replace }
