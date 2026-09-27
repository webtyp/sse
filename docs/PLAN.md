---
PLAN: "feat(sse): SSEServer mounts itself as a router.APIModule; browser client receives named events; events.Subscriber adapter"
TAG: v0.2.0
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> **Stage F1 of** `veltylabs/mjosefa-cms/docs/MASTER.md` (private repo — context restated here).
> Consumer: the chat module `github.com/veltylabs/chat_room`, which publishes a payload-less
> "you have news" event on a per-user channel and lets the browser re-fetch through its normal
> typed operations.

# Plan — `webtyp.com/sse`: mountable server, named events in the browser, `Subscriber`

## 0. Context (verified against the repo on 2026-09-26 — do not re-diagnose)

Three facts, each checked in the code:

1. **The browser client never receives what `sse.Publisher` sends.** `Publisher.Publish`
   (`events_publisher.go`) calls `PublishEvent(e.Topic, data, e.Topic)`, so every frame carries
   an `event: <topic>` line (`hub.go`, `formatSSEMessage`). The WASM client (`client.go`)
   only sets `EventSource.onmessage`. Per the HTML standard, `onmessage` fires **only** for frames
   with no `event:` field (type `"message"`); a named event is delivered solely to
   `addEventListener(<name>, …)`. So today a named event is silently dropped in the browser.
2. **`SSEServer` is not a `router.APIModule`.** Every consumer mounts it by hand:
   `r.Stream(path, server.StreamHandler())` and then chooses a gate. `webtyp.com/app` already
   wrapped that in its own `MountLogsStream` (`app/sse_adapter.go`), which is glue that belongs
   here.
3. **`SSEPublisher` (`interfaces_server.go`) is dead and wrong.** It declares
   `Publish(data []byte, channels ...string)`, but `SSEServer.Publish` is
   `Publish(data []byte, channel string)`, so `SSEServer` does **not** satisfy it. Nothing in
   `webtyp/*` or `veltylabs/*` references `sse.SSEPublisher`. (`webtyp.com/mcp` has its **own**
   `SSEPublisher`, which is a different type and is not touched.)

Not a defect (checked with a real `httpd` server, `Gzip: true`): a stream route behind the gzip
battery still delivers each flushed frame promptly. Do not "fix" gzip.

## 1. Rules for this repo

- `server.go`, `hub.go`, `events_publisher.go` are `//go:build !wasm` (backend) and may use the
  stdlib they already use (`bytes`, `sync`). Do **not** "clean up" those imports.
- `client.go`, `client_config.go` and the new `subscriber.go` are `//go:build wasm` and must use
  `webtyp.com/fmt` (never `fmt`/`errors`/`strings`/`strconv`), no `map[K]V`, no `reflect`,
  no `encoding/json`.
- Tests live in `tests/`. Run everything with `gotest` (native + browser lanes, one command);
  **both lanes must pass**. `GOOS=js GOARCH=wasm go build` compiling proves nothing.
- No `TODO`, no commented-out code, no fallback that restores old behaviour.

## 2. Design gate

**1. Prior art.**
- *Go `net/http` handlers + chi `Mount`*: a sub-app registers its own routes; the host only lists it.
  This ecosystem's `router.APIModule` (`ModelName()` + `MountAPI(r)`) is exactly that, and
  `webtyp.com/mcp`'s server already implements it.
- *Browser `EventSource`*: `addEventListener(type, fn)` is the standard way to receive named
  events; `onmessage` is only for unnamed ones.
- *Phoenix Channels / ActionCable*: a client subscribes to a topic and gets callbacks per topic.
  `events.Subscriber.Subscribe(topic, h)` already is that shape in this ecosystem.

**2. Novice-name test.** `ServerConfig.Path`, `ServerConfig.Access`, `ServerConfig.Resource`,
`SSEClient.OnEvent(name, h)`, `sse.Subscriber{Client: c}` — each reads as a sentence.
`Subscriber` mirrors the existing `sse.Publisher`.

**3. Complexity ledger.**
```
Concepts                +1 (Subscriber) −1 (dead SSEPublisher)
Files a consumer touches to mount a stream   −1 (no local MountLogsStream-style helper)
Lines at the call site  −1 per consumer (Mount(server) replaces r.Stream(...).Gate())
Ways to do the same thing  0: StreamHandler() becomes unexported in this change; MountAPI is the only way
```

**4. Where it belongs.** Here. Mounting an SSE server and adapting it to `events` are SSE
concerns; `events_publisher.go` already set the precedent for the server side.

**5. What it deletes.** `SSEPublisher` (`interfaces_server.go`); the exported
`StreamHandler()` (renamed to unexported `streamHandler()`); `webtyp/app`'s
`MountLogsStream` (done in `webtyp/app` right after this tag — see §6).

## 3. Server: `SSEServer` implements `router.APIModule`

`server_config.go` gains three fields (add them after `ChannelProvider`):

```go
// Path is the stream URL, e.g. "/events". Required.
Path string

// Access is who may open the stream. The zero value, model.AccessGuarded,
// requires an identity AND a permission on Resource (read).
// model.AccessAuthenticated requires only an identity; model.AccessPublic, none.
Access model.Access

// Resource is the permission checked when Access is model.AccessGuarded.
// Must be empty for the other two levels.
Resource model.Resource
```

`server.go`:

```go
// ModelName is the module identity (router.APIModule).
func (s *SSEServer) ModelName() string { return ModuleName }

// MountAPI registers the stream route with the configured gate.
func (s *SSEServer) MountAPI(r router.Router)

var _ router.APIModule = (*SSEServer)(nil)
```

- `const ModuleName = "sse"` in `server_config.go` (no build tag, so it is visible to both lanes).
- `MountAPI` behaviour, in this order — each failure is a `panic` with the **exact** message, the
  same way `router.ValidatePattern` already panics on a bad path at registration:
  - `Path == ""` → `panic("sse: ServerConfig.Path is required")`
  - `ChannelProvider == nil` → `panic("sse: ServerConfig.ChannelProvider is required")`
  - `Access == model.AccessGuarded && Resource == ""` →
    `panic("sse: ServerConfig.Resource is required when Access is AccessGuarded")`
  - `Access != model.AccessGuarded && Resource != ""` →
    `panic("sse: ServerConfig.Resource must be empty unless Access is AccessGuarded")`
  - then `route := r.Stream(s.config.Path, s.streamHandler())` and:
    `AccessPublic` → `route.Public()`; `AccessAuthenticated` → `route.Authenticated()`;
    `AccessGuarded` → `route.Requires(s.config.Resource, model.Read)`.
- Rename `StreamHandler()` → `streamHandler()` (unexported). Its body is unchanged, including
  the existing request-time 500 when `ChannelProvider` is nil (kept for the unit tests that call it
  through `MountAPI` on a mock router with a hand-built config).
- Fix the `ChannelProvider` doc comment in `server_config.go`: it currently says "If nil, a
  default provider is used"; there is none. New text:
  `// ChannelProvider resolves channels for each SSE connection. Required by MountAPI.`
- Delete `SSEPublisher` from `interfaces_server.go`. Keep `ChannelProvider` there.

## 4. Browser client: named events

`client.go` (`//go:build wasm`):

```go
// OnEvent registers a handler for SSE frames whose event name is name
// (frames published with PublishEvent or through sse.Publisher).
// OnMessage keeps receiving only unnamed frames. Handlers registered before
// or after Connect both work, and survive reconnects.
func (c *SSEClient) OnEvent(name string, handler func(msg *SSEMessage))
```

Implementation, exactly:
- New unexported type `eventListener struct { name string; handler func(msg *SSEMessage); fn js.Func; attached bool }`
  and field `listeners []*eventListener` on `SSEClient` (a slice, **not** a map).
- `OnEvent` appends a listener. If `c.es` is already a live `EventSource`, attach it immediately.
- `Connect` attaches **every** listener to the new `EventSource` after creating it (reconnect
  creates a new one, so listeners are re-attached there too).
- Attaching = `c.es.Call("addEventListener", name, fn)` where `fn` reads `data`, `lastEventId`
  and `type` exactly like the current `onmessage` callback, updates `c.lastEventID`, resets
  `c.reconnectAttempts`, and calls the handler with an `*SSEMessage`. Factor the shared
  "event → *SSEMessage" conversion into one unexported function used by both `onmessage` and the
  listeners; do not duplicate it.

## 5. `events.Subscriber` adapter — new file `subscriber.go`

```go
//go:build wasm

package sse

// Subscriber adapts an *SSEClient to events.Subscriber, the mirror of Publisher.
//
// It delivers the TOPIC only: the Event a handler receives always has a nil
// Payload. A subscriber cannot know which concrete model.Decodable a topic
// carries, so it does not guess; the intended use is a notification ("topic X
// changed") after which the handler fetches typed data through its normal
// router.Caller. Publish payload-less events on topics meant for the browser.
type Subscriber struct {
	Client *SSEClient
}

func (s Subscriber) Subscribe(topic string, h events.Handler) {
	s.Client.OnEvent(topic, func(*SSEMessage) { h(events.Event{Topic: topic}) })
}

var _ events.Subscriber = Subscriber{}
```

## 6. Follow-up in `webtyp/app` (not part of this dispatch)

After this tag, `webtyp/app` fails to compile against it because `StreamHandler()` is gone. That
fix is done locally, not by the executor:

- `app/start.go` and `app/daemon.go`: add `Path: LogsStreamPath, Access: model.AccessPublic` to both
  `sse.ServerConfig` literals and mount with `httpd.NewRouter(mux)` + `sseServer.MountAPI(...)`.
- Delete `MountLogsStream` from `app/sse_adapter.go`.

## 7. Tests (`tests/`)

New or changed files:

- `tests/mount_test.go` (`//go:build !wasm`), with `webtyp.com/router/mock`:
  1. `AccessPublic` → `Routes()` has one `GET` route at `Path` with `Access == model.AccessPublic`.
  2. `AccessAuthenticated` → the route's access is `model.AccessAuthenticated`, and
     `Invoke` with an empty `UserID` is rejected by the mock's gate while a non-empty one reaches
     the channel provider.
  3. `AccessGuarded` + `Resource: "events"` → route requires `"events"` / `model.Read`.
  4. Each of the four panics, asserting the exact message (`recover()` + string compare).
  5. `var _ router.APIModule = (*sse.SSEServer)(nil)` compiles (keep the assertion in
     `server.go`; the test only uses the value).
- `tests/server_test.go`: replace every `srv.StreamHandler()` call with mounting on a
  `mock.Router` and invoking the route; the assertions stay the same.
- `tests/client_test.go` (`//go:build wasm`): mock `EventSource` gets an `addEventListener`
  function that records `(name, fn)` pairs. Add:
  6. `OnEvent` before `Connect` → `addEventListener("chat_room.inbox.u1", …)` is called on
     `Connect`; dispatching a fake event object with `data`, `lastEventId`, `type` through the
     recorded fn calls the handler with those values.
  7. `OnEvent` after `Connect` attaches immediately.
  8. After a forced reconnect (`readyState = 2` + `onerror`, then the `setTimeout` callback), the
     **new** mock instance also receives `addEventListener` for every registered name.
  9. `OnMessage` does **not** fire for a named event and `OnEvent` does not fire for an unnamed
     one.
- `tests/subscriber_test.go` (`//go:build wasm`):
  10. `Subscriber{Client: c}.Subscribe("t", h)` then dispatching named event `t` calls `h` once
      with `Event{Topic: "t"}` and a nil `Payload`.
- `tests/events_publisher_test.go`: add a consumer-shaped round trip on the server lane:
  `Publisher.Publish(events.Event{Topic: "chat_room.inbox.u1"})` on a server whose provider
  resolves `["chat_room.inbox.u1"]` produces a frame containing exactly
  `event: chat_room.inbox.u1` and an empty `data:` line; a connection resolved to a different
  channel receives nothing.

## 8. Docs

- `README.md` and `docs/USAGE.md`: replace every `r.Stream(..., StreamHandler())` example with
  `httpdServer.Mount(sseServer)` / `sseServer.MountAPI(r)` and the three new config fields; add a
  "Receiving named events in the browser" section (`OnEvent`) and a "Subscriber" section with the
  nil-payload contract from §5.
- `docs/CONFIG.md`: document `Path`, `Access`, `Resource`.
- `docs/ARCHITECTURE.md`: one line that `SSEServer` is a `router.APIModule`.

## 9. Stages

| # | Stage | Files | Acceptance |
|---|---|---|---|
| S1 | Mountable server | `server_config.go`, `server.go`, `interfaces_server.go`, `tests/mount_test.go`, `tests/server_test.go` | tests 1–5 green; `grep -rn "StreamHandler()" --include=*.go .` empty; `grep -rn "SSEPublisher" --include=*.go .` empty |
| S2 | Named events in the client | `client.go`, `tests/client_test.go` | tests 6–9 green in the browser lane |
| S3 | Subscriber + round trip | `subscriber.go`, `tests/subscriber_test.go`, `tests/events_publisher_test.go` | test 10 and the round trip green |
| S4 | Docs + close | `README.md`, `docs/*.md` | `grep -rn "TODO\|FIXME\|map\[" client.go subscriber.go` empty; `gotest` green on both lanes |
