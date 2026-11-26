package live

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"
)

// StreamDecl is a typed stream: one topic, item renderer, key function and item
// class.
type StreamDecl[T any] struct {
	topic  string
	key    func(T) string
	render func(T) templ.Component
	class  string
	tail   bool
}

// DeclareStream declares a stream under topic.
func DeclareStream[T any](topic string) *StreamDecl[T] {
	validateTopic(topic)
	return &StreamDecl[T]{topic: topic}
}

// Key sets how items are keyed.
func (d *StreamDecl[T]) Key(fn func(T) string) *StreamDecl[T] { d.key = fn; return d }

// Renders sets the item body renderer.
func (d *StreamDecl[T]) Renders(fn func(T) templ.Component) *StreamDecl[T] { d.render = fn; return d }

// ItemClass sets the class on every item wrapper, in both first paint and appends.
func (d *StreamDecl[T]) ItemClass(class string) *StreamDecl[T] { d.class = class; return d }

// Tail makes live items land below the existing ones, in arrival order.
func (d *StreamDecl[T]) Tail() *StreamDecl[T] { d.tail = true; return d }

// Topic returns the stream's topic.
func (d *StreamDecl[T]) Topic() string { return d.topic }

// Item renders one keyed item with the wrapper Append sends.
func (d *StreamDecl[T]) Item(item T) templ.Component {
	key, body, err := d.parts(item)
	class := d.class
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if err != nil {
			return err
		}
		return writeItem(ctx, w, key, class, body)
	})
}

// Items renders the stream container holding items, in the order given.
func (d *StreamDecl[T]) Items(items []T) templ.Component {
	topic, tail := d.topic, d.tail
	inner := d.itemList(items)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		dir := ""
		if tail {
			dir = ` data-tc-dir="tail"`
		}
		if _, err := io.WriteString(w, `<div data-tc-stream="`+topic+`"`+dir+`>`); err != nil {
			return err
		}
		if err := inner.Render(ctx, w); err != nil {
			return err
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
}

// Append renders item and appends it for every session of w.
func (d *StreamDecl[T]) Append(w *World, item T) error {
	key, body, err := d.parts(item)
	if err != nil {
		return err
	}
	return w.Append(d.topic, key, body, ItemClass(d.class))
}

// Region renders Items(items) inside a region of the stream's topic.
func (d *StreamDecl[T]) Region(items []T) templ.Component {
	topic, inner := d.topic, d.Items(items)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return regionMarker(ctx, w, topic, inner)
	})
}

// Bind registers the stream on w. A reconnecting page catches up through a
// swap of the stream's region, so render it with Region, not Items alone.
func (d *StreamDecl[T]) Bind(w *World, src func() []T) *Stream {
	return w.Stream(d.topic, func() templ.Component { return d.Items(src()) })
}

// BindRegion registers the stream with a replay the app renders itself; the
// page must render that replay inside a region of the stream's topic.
func (d *StreamDecl[T]) BindRegion(w *World, render func() templ.Component) *Stream {
	return w.Stream(d.topic, render)
}

func (d *StreamDecl[T]) itemList(items []T) templ.Component {
	comps := make([]templ.Component, len(items))
	for i, it := range items {
		comps[i] = d.Item(it)
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, c := range comps {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
}

// A key outside the topic grammar is an error, not a panic: keys are built from data.
func (d *StreamDecl[T]) parts(item T) (string, templ.Component, error) {
	if d.key == nil || d.render == nil {
		panic(fmt.Errorf("live: stream %q needs Key and Renders before use", d.topic))
	}
	key := d.key(item)
	if err := checkTopic(key); err != nil {
		return "", nil, fmt.Errorf("live: stream %q: item key %q: %w", d.topic, key, err)
	}
	return key, d.render(item), nil
}

func writeItem(ctx context.Context, w io.Writer, key, class string, body templ.Component) error {
	if _, err := io.WriteString(w, `<div data-tc-item="`+key+`"`+appendConfig{class: class}.classAttr()+`>`); err != nil {
		return err
	}
	if body != nil {
		if err := body.Render(ctx, w); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, `</div>`)
	return err
}

// StreamFamily is a stream per entity, such as a thread per conversation.
type StreamFamily[T any] struct{ proto StreamDecl[T] }

// DeclareStreamFamily declares a family of streams under prefix.
func DeclareStreamFamily[T any](prefix string) *StreamFamily[T] {
	validateTopic(prefix)
	return &StreamFamily[T]{proto: StreamDecl[T]{topic: prefix}}
}

// Key, Renders, ItemClass and Tail configure every member.
func (f *StreamFamily[T]) Key(fn func(T) string) *StreamFamily[T] { f.proto.key = fn; return f }

// Renders sets the members' item renderer.
func (f *StreamFamily[T]) Renders(fn func(T) templ.Component) *StreamFamily[T] {
	f.proto.render = fn
	return f
}

// ItemClass sets the members' item class.
func (f *StreamFamily[T]) ItemClass(class string) *StreamFamily[T] { f.proto.class = class; return f }

// Tail makes the members tail-inserting.
func (f *StreamFamily[T]) Tail() *StreamFamily[T] { f.proto.tail = true; return f }

// Prefix returns the family's topic prefix.
func (f *StreamFamily[T]) Prefix() string { return f.proto.topic }

// Topic returns the member topic for id.
func (f *StreamFamily[T]) Topic(id string) string {
	t := f.proto.topic + "." + id
	validateTopic(t)
	return t
}

// For returns the member stream for id.
func (f *StreamFamily[T]) For(id string) *StreamDecl[T] {
	d := f.proto
	d.topic = f.Topic(id)
	return &d
}
