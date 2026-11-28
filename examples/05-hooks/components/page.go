package components

import (
	"net/http"
	"strings"
	"time"
)

// ThemeCookie names the cookie that holds the chosen theme; the theme hook
// writes it and the server reads it.
const ThemeCookie = "theme"

// themeMaxAge is how long the theme cookie lives.
const themeMaxAge = 365 * 24 * time.Hour

// Page is what every page's layout needs from the request.
type Page struct {
	// Theme is "light" or "dark".
	Theme string
	// Path is the request URI, where the no-JS theme form returns to.
	Path string
}

// PageFor reads the theme cookie and the path of r.
func PageFor(r *http.Request) Page {
	theme := "light"
	if c, err := r.Cookie(ThemeCookie); err == nil && c.Value == "dark" {
		theme = "dark"
	}
	return Page{Theme: theme, Path: r.URL.RequestURI()}
}

// SetTheme is the no-JS theme form's handler: it stores the theme cookie
// and sends the browser back to the page it came from.
func SetTheme(w http.ResponseWriter, r *http.Request) {
	theme := r.FormValue("theme")
	if theme != "light" && theme != "dark" {
		http.Error(w, "theme must be light or dark", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: ThemeCookie, Value: theme, Path: "/",
		MaxAge: int(themeMaxAge.Seconds()), SameSite: http.SameSiteLaxMode,
	})
	back := r.FormValue("back")
	// Only a same-app path: "//host" and "/\host" are other origins.
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") || strings.HasPrefix(back, "/\\") {
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// other is the theme the toggle switches to.
func other(theme string) string {
	if theme == "dark" {
		return "light"
	}
	return "dark"
}

// ThemeArgs are the theme hook's arguments; client/theme.ts declares the
// same shape by hand.
type ThemeArgs struct {
	Cookie string `json:"cookie"`
	MaxAge int    `json:"maxAge"`
}

// DialogArgs are the dialog hook's arguments; client/dialog.ts declares the
// same shape by hand.
type DialogArgs struct {
	// ID is the id of the <dialog> the hooked link opens.
	ID string `json:"id"`
}

func themeArgs() ThemeArgs {
	return ThemeArgs{Cookie: ThemeCookie, MaxAge: int(themeMaxAge.Seconds())}
}
