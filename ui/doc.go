// Package ui provides the templ components and attribute builders for templatic markup.
//
// Every component here returns a [github.com/a-h/templ.Component], or a
// [github.com/a-h/templ.Attributes] value to spread on an element, so they
// are used from .templ files as ordinary components:
//
//	<p>Count: <strong>@ui.Cell(count)</strong></p>
//
// The components render the page's first paint on the server and mark the
// elements with data-tc-* attributes. The client runtime, started by
// [github.com/nkapatos/templatic.Boot], finds those markers and keeps the
// elements in step with a [github.com/nkapatos/templatic/live.World]: it
// updates a cell's text, replaces a region's contents, adds stream items and
// turns clicks and submits into commands. The components do not talk to the
// world themselves, so each takes the live declaration it binds to, or its
// topic name. Components that name a topic or an action panic on a name
// outside [A-Za-z0-9_.-]+.
//
// # Binding state
//
//   - [Cell] shows a [live.Cell] and updates in place. [CellAttr] binds a cell
//     to an attribute (data-*, aria-*, class or value) of the element it is
//     spread on.
//   - [Bind] and [BindAttr] do the same for a session cell declared with
//     [live.DeclareSessionCell], painted with the value the page passes.
//   - [Region] marks its children as the region published under a topic; the
//     page supplies the first render, and [live.World.Redraw] or
//     [live.World.Swap] replace the contents later.
//   - A [live.Collection] renders its own container and keyed items: put it
//     in the page with @, or with [live.Collection.With] to add a class.
//
// # Raising commands
//
//   - [Button] renders a button that raises a [live.Command] with a value of
//     its input type.
//   - [Link] renders an anchor that raises the command and, without
//     JavaScript, follows its href. [Swaps] names the region it swaps and
//     overrides the one the command declared.
//   - [FormFor] renders a form that raises the command on submit and POSTs to
//     an action URL without JavaScript. It panics when the
//     input type has a field a form cannot fill. [Multipart] makes it an
//     upload form.
//
// Button, Link and FormFor take their content as templ children.
//
// # Forms after a refused POST
//
// A no-JS POST that fails validation renders the page again. Pass [Failed]
// with the request and the error from [live.Command.Decode] (or from the
// [live.Command.Form] callback) as the form's option. Inside the form, [FieldError]
// shows the message for one field and [FormError] shows errors that name no
// field. [FieldValue] and [FieldValues] return what the user typed, to put
// back into the inputs, and fall back to a default on a plain render.
//
// # Client code
//
//   - [Hook] returns attributes that attach a client hook to an element, with
//     JSON-encoded arguments.
//   - [Island] wraps a component as a client island: markup rendered on the
//     server and hydrated by a client module, named by a //templatic:island
//     pragma. Render islands before Boot, which preloads their modules.
//   - [Module] requests a client page module, once per page.
//
// # Options
//
// Components that render an element accept [Option] values: [Class] adds
// classes, [Attrs] adds attributes, and the options above ([Swaps], [Tail],
// [Multipart], [Failed]) apply where the component supports them. Attrs panics
// on data-tc-* attributes and on attributes the element owns.
//
// # Deprecated
//
// [Action], [ActionLink], [Form] and [SessionCell] and [SessionCellAttr] are
// the untyped forms taking a name or topic string. They remain for
// compatibility; use [Button], [Link], [FormFor], [Bind] and [BindAttr].
package ui
