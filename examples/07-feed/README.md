# 07 · feed

Two chat rooms and an activity log. A message posted in a room appears at
the bottom of that room on every open page; the activity log, newest on
top, gets a line for every post, for every page that connects, reconnects
or leaves, and for a server clock tick every 15 seconds. Both are live
streams: the server appends one keyed item at a time and the pages insert
it, without re-rendering the list.

```sh
npm install
go tool templatic generate
go run .                     # http://localhost:8080
```

Open it in two windows and post in one. Then turn the network off in one
window (the browser's dev tools), post in the other, and turn it back on.

## What it shows

- **A stream per room.** `live.DeclareStreamFamily[Message]("chat")` in
  `components/message.go` declares a family, with the item key
  (`Message.Key`, `m<id>`), the item renderer (`MessageView`) and `Tail`.
  `Chat.For(room)` is the room's stream, topic `chat.<room>`. Rooms are a
  fixed list checked before `For`, which panics on a name outside the topic
  grammar ([#15](https://github.com/nkapatos/templatic/issues/15)).
- **Appending.** `Chat.For(room).Append(w, m)` in `feed.go` renders the
  item and sends it to every session. A page that does not show that
  room's stream ignores it.
- **Top or bottom.** A chat stream has `Tail()`: arrivals go below the
  existing items, and the first paint is oldest first. The activity log has
  no `Tail`: arrivals go on top, so `feed.recent` returns its first paint
  newest first too. The container renders the order it is given; only live
  arrivals follow the direction.
- **The untyped layer.** The activity log uses the pieces `StreamDecl` is
  built on: `w.Stream` registers the topic and its replay, `ui.Stream` and
  `ui.StreamItem` render the container and the keyed items
  (`components/pages.templ`), and `w.Append(topic, key, comp,
  live.ItemClass(kind))` appends one, with a class per item (`post`,
  `tick`, `join`, `resume`, `leave`). `ui.Stream(topic, items, ui.Tail())`
  is the untyped `Tail`.
- **Keys.** Every item carries its key (`data-tc-item`). The runtime skips
  an arrival whose key is already on the page, so a repeated append shows
  the item once.
- **Server-side appends.** `feed.tick` is a plain goroutine; `OnConnect`,
  `OnResume` and `OnDisconnect` log the session lifecycle (`newFeed`).
- **A bounded history.** The server keeps the last 20 messages per room and
  the last 10 events (`keepMessages`, `keepEvents` in `feed.go`); a page
  load and a reconnect show only those. An open page's list is not trimmed:
  live arrivals add to it until the next load or reconnect, which cuts it
  back to the kept ones. There is no option to cap a stream on the page.
- **Posting** reuses the form pattern of [03-forms](../03-forms): one
  `Post` command, `ui.FormFor` for both transports, `Post.FormPOST` for the
  no-JS POST to `/rooms/{room}/messages`, which redirects back to the room
  with the name in the query. The `clear-on-ok` hook in `client/app.ts`
  clears the message field after a send.
- **Without JavaScript** the form posts and the page shows the room's kept
  messages and events; it does not update until the next load. Pages with
  JavaScript get a no-JS post live.
- **Testing without a browser.** `feed_test.go` posts with `livetest.Call`
  and reads the appended items with `Session.Appends`.

## New pages and reconnects

Read from `live/stream.go` and `live/session.go`, and checked in a browser
by cutting a page's connection:

- **A new page** renders the kept items into its markup and then opens the
  live connection. It gets nothing replayed: an item appended after the
  page rendered but before its connection opened does not reach it (a
  reload shows it).
- **A page that reconnects within the resume grace** (30 seconds,
  `live.WithResumeGrace`) gets each stream's current state, not the items
  it missed: the server renders the bound stream again (`Bind`'s function,
  or `w.Stream`'s) and swaps it into the region of the same topic. Appends
  sent while it was away are dropped, not queued. The page ends up with the
  kept items: missed messages appear if they are among the last 20, older
  ones on the page are cut. `OnResume` runs, not `OnConnect`.
- **The region is required for that.** The replay is a region swap, so a
  stream rendered without a region of its own topic around it (`Items`
  instead of `Region` in a `StreamDecl`, `ui.Stream` without `ui.Region`)
  keeps whatever it had, and the missed items stay missing until a reload
  (read from the code: the runtime drops a swap with no region). Both
  streams here are inside their region.
- **A page that comes back after the grace** finds its session gone: the
  server answers with a resync and the page reloads. `OnDisconnect` ran
  when the grace ran out.

## Not yet: editing and deleting

Streams only append ([#4](https://github.com/nkapatos/templatic/issues/4)).
An append with a key already on the page is skipped, so it cannot replace
an item, and nothing removes one. Editing or deleting a message would need
an upsert and a remove by key sent to every page, and the reconnect replay
to carry the result. Until then the only way to change a shown item is to
re-render the whole list as a region, which this example does not do.

## Files

| File | Holds |
|---|---|
| `main.go` | the world, the tick goroutine, the pages, the no-JS post, the routes |
| `feed.go` | the kept messages and events, the stream bindings, `post`, `log` and `tick` |
| `feed_test.go` | the append, the refusal and the bound history tested with `livetest` |
| `components/message.go` | `Message`, `Event`, the `Chat` family, `PostIn` and `Post` |
| `components/pages.templ` | the room page, the item views, the activity stream, the 404 page |
| `components/layout.templ` | the document and the room links |
| `client/app.ts`, `client/app.css` | the `clear-on-ok` hook and the styles |

`*_templ.go` files are generated by `templatic generate` and committed, as
with templ. `.templatic/` is generated and ignored.
