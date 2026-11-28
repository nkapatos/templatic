package live

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{Invalid("title", "required"), http.StatusBadRequest},
		{fmt.Errorf("save: %w", NotFound("no book 9")), http.StatusNotFound},
		{Status(http.StatusForbidden, "admins only"), http.StatusForbidden},
		{Status(200, "not an error code"), http.StatusInternalServerError},
		{errors.New("disk full"), http.StatusInternalServerError},
	} {
		if got := HTTPStatus(tc.err); got != tc.want {
			t.Errorf("HTTPStatus(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}
