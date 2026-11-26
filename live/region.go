package live

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"

	"github.com/a-h/templ"
)

// Region is a server-rendered fragment the world can redraw.
type Region struct {
	world  *World
	name   string
	render func() templ.Component
}

// Name returns the region's topic.
func (r *Region) Name() string { return r.name }

// Render writes the region's current markup inside its targeting marker.
func (r *Region) Render(ctx context.Context, w io.Writer) error {
	return regionMarker(ctx, w, r.name, r.render())
}

func regionMarker(ctx context.Context, w io.Writer, topic string, comp templ.Component) error {
	if _, err := io.WriteString(w, `<div data-tc-region="`+topic+`">`); err != nil {
		return err
	}
	if comp != nil {
		if err := comp.Render(ctx, w); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, `</div>`)
	return err
}

func (r *Region) redraw() (string, error) {
	comp := r.render()
	if comp == nil {
		return "", fmt.Errorf("live: region %q renders nil", r.name)
	}
	return renderComponent(comp)
}

// Swap renders comp and pushes it to one session as topic's markup.
func (w *World) Swap(s *Session, topic string, comp templ.Component) error {
	validateTopic(topic)
	if s == nil {
		return fmt.Errorf("live: swap on %q has no session", topic)
	}
	if comp == nil {
		return fmt.Errorf("live: swap on %q renders nil", topic)
	}
	html, err := renderComponent(comp)
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		HTML string `json:"html"`
	}{HTML: html})
	if err != nil {
		return err
	}
	s.offer(event{kind: KindSwap, topic: topic, body: body})
	return nil
}

func renderComponent(comp templ.Component) (string, error) {
	var buf bytes.Buffer
	buf.Grow(4096)
	if err := comp.Render(context.Background(), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
