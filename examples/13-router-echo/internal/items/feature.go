// Package items is the list feature: its command, its live state and the
// handlers the app mounts on its router.
package items

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/nkapatos/templatic/live"

	"examples/router-echo/internal/page"
)

// LikeIn is the input of Like. The form carries id as a hidden field for
// the runtime's request; on a no-JS POST the route's {id} wins over it.
type LikeIn struct {
	ID string `json:"id"`
}

// Validate refuses an empty id.
func (in LikeIn) Validate() error {
	if in.ID == "" {
		return live.Invalid("id", "no item")
	}
	return nil
}

// Like adds one like to an item.
var Like = live.NewCommand[LikeIn]("item.like")

// Feature holds the handlers the app routes; each reads {id} with
// r.PathValue, so the router must set it.
type Feature struct {
	List     http.Handler // GET /
	Show     http.Handler // GET /items/{id}
	LikeForm http.Handler // POST /items/{id}/like, Like without JavaScript
}

// Register declares the likes on w, handles Like and returns the handlers.
func Register(w *live.World) *Feature {
	likes := live.NewCellFamily(w, "likes", 0)
	like := func(id string) error {
		if _, ok := find(id); !ok {
			return live.NotFound("no item " + id)
		}
		likes.Get(id).Update(func(n int) int { return n + 1 })
		return nil
	}
	Like.Handle(w, func(_ *live.Session, in LikeIn) (any, error) {
		return nil, like(in.ID)
	})
	return &Feature{
		List: templ.Handler(List(catalog, likes)),
		Show: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			id := r.PathValue("id")
			it, ok := find(id)
			if !ok {
				templ.Handler(page.NotFound("There is no item "+id+"."), templ.WithStatus(http.StatusNotFound)).ServeHTTP(rw, r)
				return
			}
			templ.Handler(Detail(it, likes.Get(id))).ServeHTTP(rw, r)
		}),
		LikeForm: Like.FormPOST(w, func(_ *http.Request, in LikeIn) (string, error) {
			return "/items/" + in.ID, like(in.ID)
		}, nil),
	}
}
