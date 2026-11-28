package components

import (
	"context"
	"slices"
	"strconv"

	"github.com/nkapatos/templatic/ui"
)

// checked reports whether a checkbox is ticked: by the submitted values after
// a refused POST (an unticked box sends nothing), else by fallback.
func checked(ctx context.Context, name, value string, fallback []string) bool {
	return slices.Contains(ui.FieldValues(ctx, name, fallback), value)
}

func favoriteValues(b Book) []string {
	if b.Favorite {
		return []string{"true"}
	}
	return nil
}

func pagesText(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
