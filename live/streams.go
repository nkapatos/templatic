package live

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"html"
	"io"

	"github.com/a-h/templ"
)

// Stream is a declared stream topic.
type Stream struct {
	world  *World
	name   string
	render func() templ.Component
}

// Name returns the stream's topic.
func (s *Stream) Name() string { return s.name }

// Stream declares, or re-declares, a stream under topic.
func (w *World) Stream(topic string, render func() templ.Component) *Stream {
	validateTopic(topic)
	w.mu.Lock()
	defer w.mu.Unlock()
	if s, ok := w.streams[topic]; ok {
		s.render = render
		return s
	}
	s := &Stream{world: w, name: topic, render: render}
	w.streams[topic] = s
	return s
}

// Append renders comp as one keyed item and appends it under topic for every
// session.
func (w *World) Append(topic, key string, comp templ.Component, opts ...AppendOption) error {
	validateTopic(topic)
	var cfg appendConfig
	for _, o := range opts {
		o(&cfg)
	}
	if err := checkTopic(key); err != nil {
		return fmt.Errorf("live: append on %q: item key %q: %w", topic, key, err)
	}
	if comp == nil {
		return fmt.Errorf("live: append on %q renders nil item %q", topic, key)
	}
	itemHTML, err := renderComponent(templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return writeItem(ctx, w, key, cfg.class, comp)
	}))
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		Items []streamItem `json:"items"`
	}{Items: []streamItem{{
		K:    key,
		HTML: itemHTML,
	}}})
	if err != nil {
		return err
	}
	w.publish(event{kind: KindAppend, topic: topic, body: body})
	return nil
}

// A render failure skips the stream; a broken frame would not be repaired.
func (s *Stream) replay(sess *Session) error {
	if s.render == nil {
		return nil
	}
	html, err := renderComponent(s.render())
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		HTML string `json:"html"`
	}{HTML: html})
	if err != nil {
		return err
	}
	sess.offer(event{kind: KindSwap, topic: s.name, body: body})
	return nil
}

func (w *World) replayStreams(s *Session) {
	w.mu.RLock()
	streams := make([]*Stream, 0, len(w.streams))
	for _, st := range w.streams {
		streams = append(streams, st)
	}
	w.mu.RUnlock()
	for _, st := range streams {
		_ = st.replay(s)
	}
}

// AppendOption configures the wrapper of one appended item.
type AppendOption func(*appendConfig)

type appendConfig struct{ class string }

// ItemClass sets the class on the appended item's wrapper.
func ItemClass(class string) AppendOption {
	return func(c *appendConfig) { c.class = class }
}

func (c appendConfig) classAttr() string {
	if c.class == "" {
		return ""
	}
	return ` class="` + html.EscapeString(c.class) + `"`
}
