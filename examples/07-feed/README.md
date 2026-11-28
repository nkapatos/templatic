# 07 · feed

Two chat rooms and an activity log. A message posted in a room appears at
the bottom of that room on every page that shows it; a like updates it in
place and a delete removes it, everywhere. The activity log, newest on top,
gets a line for every post, for every page that connects, reconnects or
leaves, and for a server clock tick every 15 seconds. Both are live
collections: the server sends one numbered change at a time and the pages
apply it by key, without re-rendering the list.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

Open it in two windows and post, like and delete in one. Then turn the
network off in one window (the browser's dev tools), post and delete in the
other, and turn it back on.

## What it shows

- **A collection per room.** `live.NewCollectionFamily(w, "chat",
  Message.Key)` in `feed.go`, with the item renderer (`MessageView`), the
  elements (`Elements("ol", "li")`: the room is an ordered list of `<li>`),
  `Tail` and `Limit`. `rooms.Get(room)` is the room's collection, topic
  `chat.<room>`; it returns an error for a name outside the topic grammar.
  The collections hold the messages: there is no store beside them.
- **Rendering.** `@chat` in `components/pages.templ` renders the
  container, its items and the version they are at (`data-tc-ver`). The
  page passes the collection as a component.
- **Changes.** `Upsert` adds a message (`post`), `Update` changes one
  atomically (`like`: two likes at once both count), `Remove` deletes one
  (`remove`). Each is one change, sent only to pages that show that room:
  a page on #random gets nothing from #general.
- **Top or bottom.** A room has `Tail()`: new messages go at the bottom.
  The activity log has none: new events go on top. The order on the server
  and on every page is the same.
- **A limit on both sides.** `Limit(keepMessages)` keeps the last 20
  messages per room on the server, and the container carries the limit
  (`data-tc-limit`), so an open page drops the oldest as new ones arrive
  too.
- **A class per item.** `ItemAttrs` sets each activity line's class from
  its kind (`post`, `tick`, `join`, `resume`, `leave`).
- **Server-side changes.** `feed.tick` is a plain goroutine; `OnConnect`,
  `OnResume` and `OnDisconnect` log the session lifecycle (`newFeed`).
- **Forms in live items.** Every message carries a like and a delete form
  (`ui.FormFor`), live-inserted with the message. They POST to
  `/rooms/{room}/messages/{id}/like` or `/delete` (`Like.Form` and
  `Delete.Form` in `newMux`, `main.go`); without JavaScript the browser is
  redirected back. The room and the id come from the route. A form that
  arrives in a live item has no run-once submission id
  ([#32](https://github.com/nkapatos/templatic/issues/32)), so a repeated
  like counts twice.
- **Posting** reuses the form pattern of [03-forms](../03-forms): one
  `Post` command, `ui.FormFor` and `Post.Form` for both transports at
  `/rooms/{room}/messages`; without JavaScript it redirects back to the
  room with the name in the query. The `clear-on-ok` hook in `client/app.ts`
  clears the message field after a send.
- **Typing is safe.** A like re-renders its message on every page; a field
  someone is typing in keeps its value and focus.
- **Without JavaScript** the forms post and the page shows the room's kept
  messages and events; it does not update until the next load. Pages with
  JavaScript get no-JS changes live.
- **Testing without a browser.** `feed_test.go` shows a room to a
  `livetest` session (`Show`) and reads the changes with `Ops`.

## New pages and reconnects

- **A new page** renders the room at a version and tells the server that
  version when its live connection opens. A message posted in between
  arrives then.
- **A page that reconnects within the resume grace** (30 seconds,
  `live.WithResumeGrace`) gets the changes it missed, in order, from the
  version its markup holds: missed messages appear, deleted ones go. If it
  was away longer than the server's history of recent changes reaches
  (`Collection.History`, 64 KB by default), it gets the whole room instead.
  `OnResume` runs, not `OnConnect`.
- **A page that comes back after the grace** finds its session gone: the
  server answers with a resync and the page reloads. `OnDisconnect` ran
  when the grace ran out.
- **Scrolling.** Chromium and Firefox keep a scrolled list in place when
  items arrive above the view (scroll anchoring); Safari does not, so a
  scrolled head-insert list jumps there.

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the tick goroutine, the pages, the no-JS post, like and delete, the routes |
| `feed.go` | the room collections and the activity log, `post`, `like`, `remove`, `log` and `tick` |
| `feed_test.go` | reach, like and delete, the refusal and the limit, tested with `livetest` |
| `components/message.go` | `Message`, `Event`, `PostIn`, `MessageIn`, and the `Post`, `Like` and `Delete` commands |
| `components/pages.templ` | the room page, the item views with their forms, the 404 page |
| `components/layout.templ` | the document and the room links |
| `client/app.ts`, `client/app.css` | the `clear-on-ok` hook and the styles |

`*_templ.go` files are generated by `templatic generate` and committed, as
with templ. `.templatic/` is generated and ignored.
