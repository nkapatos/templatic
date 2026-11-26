package live

import (
	"strings"
	"testing"
	"time"
)

func TestDevFrameReachesSessions(t *testing.T) {
	w := NewWorld()
	ts := serveWorld(t, w)
	r := openStream(t, ts, "dev-sid")
	defer r.Close()
	r.discardUntilReady(t)

	w.Dev(DevFrame{What: "css", CSS: map[string]string{"/d/app-a.css": "/d/app-b.css"}})
	w.Dev(DevFrame{What: "reload"})
	for _, want := range []string{
		`{"kind":"dev","what":"css","css":{"/d/app-a.css":"/d/app-b.css"}}`,
		`{"kind":"dev","what":"reload"}`,
	} {
		got, ok := r.next(t, 2*time.Second)
		if !ok || !strings.Contains(got, want) {
			t.Fatalf("frame %q, want %q", got, want)
		}
	}
}
