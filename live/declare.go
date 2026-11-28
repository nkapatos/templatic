package live

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/a-h/templ"

	"github.com/nkapatos/templatic/internal/form"
)

// Command is a typed command declaration: an action name bound to input type In.
type Command[In any] struct {
	name  string
	swaps string
}

// NewCommand declares a command; it panics on a name outside [A-Za-z0-9_.-]+.
func NewCommand[In any](name string) *Command[In] {
	validateTopic(name)
	return &Command[In]{name: name}
}

// Swaps declares the session region this command swaps and returns c.
func (c *Command[In]) Swaps(topic string) *Command[In] {
	validateTopic(topic)
	c.swaps = topic
	return c
}

// Name returns the action name.
func (c *Command[In]) Name() string { return c.name }

// SwapTopic returns the region declared with Swaps, or "".
func (c *Command[In]) SwapTopic() string { return c.swaps }

// Handle registers fn as the command's handler on w.
func (c *Command[In]) Handle(w *World, fn func(s *Session, in In) (any, error)) {
	w.Command(c.name, Handle(fn))
}

// Decode reads the command's input from a no-JS form POST and runs Validate.
func (c *Command[In]) Decode(r *http.Request) (In, error) { return DecodeForm[In](r) }

// DecodeForm decodes a form POST into In and runs In.Validate; route wildcards win
// over form fields.
func DecodeForm[In any](r *http.Request) (In, error) {
	// ParseForm first: for a urlencoded body ParseMultipartForm hides ParseForm's error.
	err := r.ParseForm()
	if err == nil {
		if err = r.ParseMultipartForm(32 << 20); err == http.ErrNotMultipart {
			err = nil
		}
	}
	if err != nil {
		var in In
		if tooLarge(err) {
			return in, Status(http.StatusRequestEntityTooLarge, "request body too large")
		}
		return in, &InputError{Message: "malformed form"}
	}
	vals := make(url.Values, len(r.PostForm))
	for name, v := range r.PostForm {
		vals[name] = v
	}
	// Asking per field keeps this independent of the router.
	for _, name := range form.Names(reflect.TypeFor[In]()) {
		if v := r.PathValue(name); v != "" {
			vals[name] = []string{v}
		}
	}
	return decodeInput[In](formArgs(vals))
}

// FormPOST serves the command's no-JS form POST: it answers 303 to the URL fn
// returns, and input errors go to invalid (400 when nil). A repeat of a form's
// submission id answers the first run's outcome without calling fn again.
func (c *Command[In]) FormPOST(
	w *World,
	fn func(r *http.Request, in In) (redirect string, err error),
	invalid func(w http.ResponseWriter, r *http.Request, err error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		in, err := c.Decode(r)
		if err != nil {
			formOutcome(rw, r, "", err, invalid)
			return
		}
		claims, ok := w.verifySubmit(r.PostForm.Get(form.SubmitField))
		if !ok {
			to, err := fn(r, in)
			formOutcome(rw, r, to, err, invalid)
			return
		}
		sub, first := w.claimSubmit(claims)
		if !first {
			sub.replayForm(rw, r)
			return
		}
		settled := false
		defer func() {
			if !settled {
				sub.settle(false)
			}
		}()
		to, err := fn(r, in)
		sub.redirect = to
		settled = true
		sub.settle(err == nil)
		formOutcome(rw, r, to, err, invalid)
	})
}

func formOutcome(w http.ResponseWriter, r *http.Request, to string, err error, invalid func(http.ResponseWriter, *http.Request, error)) {
	switch {
	case err == nil:
		http.Redirect(w, r, to, http.StatusSeeOther)
	case len(InputErrors(err)) > 0:
		if invalid != nil {
			invalid(w, r, err)
			return
		}
		http.Error(w, inputText(err), http.StatusBadRequest)
	case writeStatusError(w, err):
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func inputText(err error) string {
	var lines []string
	for _, ie := range InputErrors(err) {
		if ie.Field == "" {
			lines = append(lines, ie.Message)
		} else {
			lines = append(lines, ie.Field+": "+ie.Message)
		}
	}
	return strings.Join(lines, "\n")
}

// SessionCellDecl declares a session-scoped cell with its topic, type and initial
// value.
type SessionCellDecl[T any] struct {
	topic   string
	initial T
}

// DeclareSessionCell declares a session cell that starts at initial unless a
// page-token seed set one.
func DeclareSessionCell[T any](topic string, initial T) *SessionCellDecl[T] {
	validateTopic(topic)
	return &SessionCellDecl[T]{topic: topic, initial: initial}
}

// Topic returns the cell's topic.
func (d *SessionCellDecl[T]) Topic() string { return d.topic }

// Initial returns the declared initial value.
func (d *SessionCellDecl[T]) Initial() T { return d.initial }

// For returns s's cell, creating it on first use.
func (d *SessionCellDecl[T]) For(s *Session) *Cell[T] {
	if s == nil {
		panic(fmt.Errorf("live: session cell %q needs a session", d.topic))
	}
	return SessionCell(s.world, s, d.topic, d.initial)
}

// Get returns s's value of the cell.
func (d *SessionCellDecl[T]) Get(s *Session) T { return d.For(s).Get() }

// Set stores v in s's cell and publishes it to s.
func (d *SessionCellDecl[T]) Set(s *Session, v T) { d.For(s).Set(v) }

// Update applies fn to s's cell atomically and publishes the result.
func (d *SessionCellDecl[T]) Update(s *Session, fn func(old T) T) { d.For(s).Update(fn) }

// Seed presets the cell for the session a page token mints.
func (d *SessionCellDecl[T]) Seed(v T) TokenOption { return SeedCell(d.topic, v) }

// SessionValueDecl declares a server-only session value.
type SessionValueDecl[T any] struct {
	key     string
	initial func() T
}

// DeclareSessionValue declares a session value; initial is called once per session.
func DeclareSessionValue[T any](key string, initial func() T) *SessionValueDecl[T] {
	validateTopic(key)
	if initial == nil {
		initial = func() T { var zero T; return zero }
	}
	return &SessionValueDecl[T]{key: key, initial: initial}
}

// For returns s's value, creating it on first use.
func (d *SessionValueDecl[T]) For(s *Session) *Value[T] {
	if s == nil {
		panic(fmt.Errorf("live: session value %q needs a session", d.key))
	}
	s.cellMu.Lock()
	existing, ok := s.values[d.key].(*Value[T])
	s.cellMu.Unlock()
	if ok {
		return existing
	}
	return SessionValue(s.world, s, d.key, d.initial())
}

// Get returns s's value.
func (d *SessionValueDecl[T]) Get(s *Session) T { return d.For(s).Get() }

// Set stores x as s's value.
func (d *SessionValueDecl[T]) Set(s *Session, x T) { d.For(s).Set(x) }

// Update applies fn to s's value atomically.
func (d *SessionValueDecl[T]) Update(s *Session, fn func(old T) T) { d.For(s).Update(fn) }

// SessionRegion is a per-session region: one topic and one renderer over view model
// M.
type SessionRegion[M any] struct {
	topic  string
	render func(M) templ.Component
}

// DeclareSessionRegion declares a session region under topic.
func DeclareSessionRegion[M any](topic string) *SessionRegion[M] {
	validateTopic(topic)
	return &SessionRegion[M]{topic: topic}
}

// Renders binds the region's renderer and returns r.
func (r *SessionRegion[M]) Renders(fn func(M) templ.Component) *SessionRegion[M] {
	r.render = fn
	return r
}

// Topic returns the region's topic.
func (r *SessionRegion[M]) Topic() string { return r.topic }

// Render returns the region's marker with m's markup as the first paint.
func (r *SessionRegion[M]) Render(m M) templ.Component {
	render := r.mustRender()
	topic := r.topic
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return regionMarker(ctx, w, topic, render(m))
	})
}

// Swap renders m and pushes it to s as the region's latest markup.
func (r *SessionRegion[M]) Swap(s *Session, m M) error {
	if s == nil {
		return fmt.Errorf("live: swap on %q has no session", r.topic)
	}
	return s.world.Swap(s, r.topic, r.mustRender()(m))
}

// Refresh re-renders the region for every session from model; model returns false
// to skip a session.
func (r *SessionRegion[M]) Refresh(w *World, model func(s *Session) (M, bool)) error {
	render := r.mustRender()
	var first error
	for _, s := range w.liveSessions() {
		// stamped before the model is read, so a swap of newer state made meanwhile wins
		gen := w.swapGen.Add(1)
		m, ok := model(s)
		if !ok {
			continue
		}
		if err := w.swap(s, r.topic, render(m), gen); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (r *SessionRegion[M]) mustRender() func(M) templ.Component {
	if r.render == nil {
		panic(fmt.Errorf("live: region %q has no renderer: call Renders before Render or Swap", r.topic))
	}
	return r.render
}
