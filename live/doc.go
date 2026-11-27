// Package live holds the server-side state of a templatic app and pushes it to browsers.
//
// A [World] is the state a set of pages shares. Pages render the current
// state on the server; each browser then opens a Server-Sent Events (SSE)
// connection and receives updates as the state changes. Browsers send user
// actions back as commands, ordinary POST requests. The templ components that
// bind markup to this state are in [github.com/nkapatos/templatic/ui]; the
// root package [github.com/nkapatos/templatic] serves the client bundle and
// wires a world into an app.
//
// # World
//
// [NewWorld] creates an empty world. Options tune the heartbeat
// ([WithHeartbeat]), the coalescing of bursts of updates ([WithCoalesceWindow]),
// how long a session survives a dropped connection ([WithResumeGrace]), the
// page-token key and lifetime ([WithKey], [WithTokenTTL]) and which request
// headers and cookies handlers may read ([WithRequestHeaders],
// [WithRequestCookies]). All state is declared against a world and addressed
// by a topic, a name matching [A-Za-z0-9_.-]+. Declaring a topic with any
// other name panics.
//
// A world is mounted on a mux with [World.Mount]. It registers GET base/stream
// ([World.SSE]) and POST base/commands ([World.Commands]); the two handlers
// can also be mounted separately.
//
// # Shared state
//
// A [Cell] is a typed, versioned value shared by every session. [NewCell]
// declares it; [Cell.Set] and [Cell.Update] store a new value and publish
// it to every connected session. A [CellFamily] is a keyed set of such cells
// under one topic prefix, for one cell per entity.
//
// A cell is published whole: each change sends the entire value to every
// connected session, whether or not its page shows the cell, and every new
// stream starts with every world cell. That suits small values, such as a
// count, a status or a short list. A dataset that changes item by item
// belongs in a [Collection], whose changes cost the changed items and reach
// only the pages that show it.
//
// A [Region] is a server-rendered fragment. [World.Region] declares it with a
// render function, and [World.Redraw] re-renders it and pushes the markup to
// all sessions. [World.Swap] pushes markup for a topic to one session.
//
// A [Collection] is a keyed, ordered set of items the world holds.
// [NewCollection] declares it with a key function;
// [Collection.Upsert] and [Collection.Remove] change it, and each change is
// published once, as a numbered op, to the sessions whose page shows the
// collection. The page carries the version it was rendered at, in the
// container or the snapshot below, so a page that missed ops, between render and connect or while its stream
// was down, gets them from a bounded history ([Collection.History]), or the
// whole set once the history no longer reaches back. A [CollectionFamily]
// holds one collection per entity.
//
// A collection with [Collection.Renders] is rendered by the server: its
// container comes from [Collection.Render] and its ops carry each item's
// markup. Without it, it is a data collection: an island props field of type
// [Snapshot], from [Collection.Snapshot] and bound to the collection's
// topic, seeds the page, and its ops carry each item as JSON.
//
// # Session state
//
// A [Session] is one browser page connected to the world. State private to a
// session is declared once and then looked up per session:
//
//   - [DeclareSessionCell] is a cell published only to its session. A
//     page token can seed its first value with [SessionCellDecl.Seed].
//   - [DeclareSessionRegion] is a region rendered from a per-session view
//     model, pushed with [SessionRegion.Swap] or to all sessions with
//     [SessionRegion.Refresh].
//   - [DeclareSessionValue] is a server-only value that is never published.
//
// [World.OnConnect], [World.OnResume] and [World.OnDisconnect] register
// callbacks for the session lifecycle. [Session.Params], [Session.URL],
// [Session.Request] and [Session.CommandRequest] expose what the page token
// attested and what the requests carried.
//
// # Commands
//
// A [Command] binds an action name to an input type. Declare it once,
// register its handler on a world, and let the page raise it:
//
//	type AddIn struct {
//		N int `json:"n"`
//	}
//
//	func (in AddIn) Validate() error {
//		if in.N < -100 || in.N > 100 {
//			return live.Invalid("n", "pick a step between -100 and 100")
//		}
//		return nil
//	}
//
//	var Add = live.NewCommand[AddIn]("count.add")
//
//	Add.Handle(w, func(s *live.Session, in AddIn) (any, error) {
//		count.Update(func(v int) int { return v + in.N })
//		return nil, nil
//	})
//
// The handler's result is sent to the client; nil answers 204, and [At]
// tells the client that the command moved the session to a new URL.
// [Command.Swaps] names the session region a command swaps. The untyped form
// is [World.Command] with a [CommandHandler]; [Handle] adapts a typed
// function to it.
//
// The same command serves browsers without JavaScript. [Command.FormPOST]
// returns an [net/http.Handler] for the form's POST: it decodes the fields,
// runs the handler's function and answers 303 to the URL that function
// returns. Input errors go to a callback that usually renders the page again,
// where ui.Failed shows them. A repeated submission with the same submission
// id ([World.SubmitID]) is answered with the first outcome and does not run
// again.
//
// # Input and validation
//
// Command input arrives as JSON or as submitted form fields; both decode into
// the In type. If In, or *In, implements [Validator], Validate runs after
// decoding, so one submit reports every problem. A refusal is an
// [InputError], built with [Invalid] or collected from several fields with
// [errors.Join]; [InputErrors] lists the ones in an error. The client gets a
// 400 response naming each field. Other handler errors may use [Status] or
// [NotFound] to choose an HTTP status. [DecodeForm] and [Command.Decode]
// decode and validate a form POST by hand. For uploads, [World.SessionFor]
// finds the session of a multipart request and [WriteInvalid] answers a
// refused file.
//
// # Page tokens
//
// A page token is an HMAC-signed claim that the page the server rendered may
// open a session. [World.PageToken] mints one for a request, optionally with
// [TokenParam] attestations and [SeedCell] presets; the client sends it when
// it connects, and the world verifies it before creating the session. Set the
// key with [WithKey] (32 or more random bytes) in production. Without a key
// the world uses a per-process development key and logs a warning, so pages
// must reload after a restart. Tokens expire after [DefaultTokenTTL] unless
// [WithTokenTTL] says otherwise. Apps normally let
// [github.com/nkapatos/templatic.WithPageTokens] mint tokens instead of calling
// PageToken directly.
//
// # Wire
//
// The stream carries events of the kinds KindCell, KindSwap, KindOps,
// KindReset, KindReady, KindResync and KindDev. Cell events carry a value and
// swap events markup; ops and reset events carry a collection's changes or
// its whole set at a version, as markup or as JSON items. Bursts of publishes are coalesced into one
// flush. A session that loses its connection keeps its place for the resume
// grace; a client that reconnects too late, whose session the server no
// longer holds, or that cannot keep up receives a resync event instead of
// incremental updates. [World.Dev] sends a development
// notice to every session and is for the dev loop only.
//
// Test code can drive a world without a network using
// [github.com/nkapatos/templatic/live/livetest].
package live
