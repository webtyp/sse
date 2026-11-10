---
PLAN: "refactor: the reconnect delay comes from webtyp.com/backoff instead of a private copy"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 8356451411321901358
PR: https://github.com/webtyp/sse/pull/8
REVIEWED_COMMIT: 0481358f45042b33e048f7d81aea9a2204b8757b
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work. Commit after every stage.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.
> **Publishing:** you publish only by OPENING THE PULL REQUEST (a `git push` from your sandbox does not reach GitHub, and work not submitted is lost when the session ends).
> **Never ask a yes/no question about progress**; the answer is always yes. Ask only when a signature in this plan is wrong, and name the symbol.
> Never write `go.mod`/`go.sum` lines by hand: only `go get` and `go mod tidy` do. No scratch files and no scripts over the source.

# Plan: `sse` uses `webtyp.com/backoff` for the reconnect delay

## Why (verified against the repo — do not re-diagnose)

`client.go:152-160` (`SSEClient.reconnect`, `//go:build wasm`) computes the wait before reconnecting by hand:

```go
delay := c.config.RetryInterval * (1 << c.reconnectAttempts)
if delay > c.config.MaxRetryDelay {
	delay = c.config.MaxRetryDelay
}
if delay <= 0 {
	delay = 1000 // Default 1s if misconfigured
}
```

The ecosystem has one library for exactly this, `webtyp.com/backoff` (published `v0.2.0`; four repos had a private copy, this is one). The same logic never lives in two repos: this plan deletes the copy and imports the library. The public fields `ClientConfig.RetryInterval` and `ClientConfig.MaxRetryDelay` (milliseconds, `int`) **do not change** (no consumer changes); the client converts them inside.

**Already in the working tree, part of this delivery, do NOT revert or touch:** `AGENTS.md` (the `reflect` row says `definitionc`), `models.go` renamed to `definition.go`, `models_orm.go` renamed to `definition_gen.go`.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go get webtyp.com/backoff@v0.2.0 webtyp.com/time@v0.6.5
go mod tidy
```
`GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`; no `replace` in `go.mod`. If a module cannot be fetched, stop and say so in the PR; never invent a hash.

## APIs used (read at the published tags; if a signature differs, stop and name the symbol in the PR)

| Symbol | Exact signature | Tag | File |
|---|---|---|---|
| `backoff.New` | `func New(first, longest time.Duration) (Policy, error)` — `time` is `webtyp.com/time`; `first` must be > 0 and `longest` ≥ `first` | backoff v0.2.0 | `backoff.go` |
| `Policy.DelayAfter` | `func (p Policy) DelayAfter(failures int64) (time.Duration, error)` — `failures` is how many have happened (1 → `first`, 2 → `2×first`, …, never above `longest`); a zero `Policy` answers `ErrPolicyNotCreated` | backoff v0.2.0 | `backoff.go` |
| `backoff.ErrFirstDelayNotPositive`, `backoff.ErrLongestBelowFirst` | typed string constants, comparable with `==`; each message says what happened, whether data is harmed and what to do | backoff v0.2.0 | `backoff.go` |
| `time.Duration`, `time.Millisecond` | `type Duration int64` (nanoseconds); `Millisecond Duration`; `(Duration) Milliseconds() int64` | time v0.6.5 | `api.go` |

## Stage 1 — `client.go` and `client_config.go`

1. `client_config.go`: update the two field comments only: `RetryInterval`: «first wait in milliseconds before reconnecting; must be greater than zero» and `MaxRetryDelay`: «longest wait in milliseconds; must be at least RetryInterval». Add to the struct comment: «An invalid pair is reported through OnError and the client does not reconnect.» The file keeps its `//go:build wasm` line and gets no new import or constant.
2. `client.go`: replace the whole `reconnect` method with exactly the code below, and add the imports `"webtyp.com/backoff"` and `"webtyp.com/time"` (keep `syscall/js` and `webtyp.com/fmt`):

```go
func (c *SSEClient) reconnect() {
	c.Close()

	if c.config.MaxReconnectAttempts > 0 && c.reconnectAttempts >= c.config.MaxReconnectAttempts {
		if c.errorHandler != nil {
			c.errorHandler(fmt.Err("max reconnect attempts reached"))
		}
		return
	}

	delay, err := c.reconnectDelay()
	if err != nil {
		if c.errorHandler != nil {
			c.errorHandler(err)
		}
		return
	}

	js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		c.Connect()
		return nil
	}), delay)

	c.reconnectAttempts++
}

// reconnectDelay answers, in milliseconds, how long to wait before the next reconnection: the delay doubles after each
// failed attempt, from RetryInterval up to MaxRetryDelay (the policy is webtyp.com/backoff). An invalid pair of values
// is an error: it is never replaced by a default.
func (c *SSEClient) reconnectDelay() (int, error) {
	policy, err := backoff.New(time.Duration(c.config.RetryInterval)*time.Millisecond, time.Duration(c.config.MaxRetryDelay)*time.Millisecond)
	if err != nil {
		return 0, err
	}
	wait, err := policy.DelayAfter(int64(c.reconnectAttempts) + 1)
	if err != nil {
		return 0, err
	}
	return int(wait.Milliseconds()), nil
}
```
   (`reconnectAttempts` starts at 0 and counts the failures already retried, so the first reconnection is failure number 1: `DelayAfter(1)` = `RetryInterval`, exactly what `RetryInterval << 0` was.)

**One behaviour change, on purpose:** before, `RetryInterval <= 0` or `MaxRetryDelay < RetryInterval` silently reconnected after 1 second («Default 1s if misconfigured»); now the error of `backoff` goes to `OnError` and the client does not reconnect. A silent fallback hides a wrong config.

**Deletions** (grep-verifiable): `grep -n "1 << c.reconnectAttempts\|Default 1s" *.go` → empty.

## Stage 2 — tests (`tests/reconnect_test.go`, new, `//go:build wasm`, package `sse_test`)

Reuse the helpers of `tests/client_test.go` (`setupMockEventSource`, same package). Each test replaces `setTimeout` with a double that records `args[1].Int()` (the delay) and does NOT call the callback. Written exactly:

```go
//go:build wasm

package sse_test

import (
	"syscall/js"
	"testing"

	"webtyp.com/backoff"
	. "webtyp.com/sse"
)

// recordDelays replaces setTimeout with a double that records each delay and never runs the callback.
func recordDelays() *[]int {
	delays := &[]int{}
	js.Global().Set("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		*delays = append(*delays, args[1].Int())
		return nil
	}))
	return delays
}

// failOnce simulates a lost connection (readyState 2) on the EventSource es.
func failOnce(es js.Value) {
	es.Set("readyState", 2)
	es.Get("onerror").Invoke()
}

func TestClientReconnect_DelayDoublesUpToTheCap(t *testing.T) {
	getInstances := setupMockEventSource()
	delays := recordDelays()
	client := New(&Config{}).Client(&ClientConfig{Endpoint: "/events", RetryInterval: 10, MaxRetryDelay: 50})
	client.Connect()
	es := getInstances()[0]

	for i := 0; i < 4; i++ {
		failOnce(es)
	}

	want := []int{10, 20, 40, 50}
	if len(*delays) != len(want) {
		t.Fatalf("delays = %v, want %v", *delays, want)
	}
	for i := range want {
		if (*delays)[i] != want[i] {
			t.Fatalf("delays = %v, want %v", *delays, want)
		}
	}
}

func TestClientReconnect_InvalidConfigIsReportedNotRetried(t *testing.T) {
	getInstances := setupMockEventSource()
	delays := recordDelays()
	client := New(&Config{}).Client(&ClientConfig{Endpoint: "/events"}) // RetryInterval 0: invalid
	var errs []error
	client.OnError(func(err error) { errs = append(errs, err) })
	client.Connect()

	failOnce(getInstances()[0])

	if len(*delays) != 0 {
		t.Fatalf("an invalid config must not schedule a reconnection, got delays %v", *delays)
	}
	if len(errs) != 2 || errs[1] != backoff.ErrFirstDelayNotPositive {
		t.Fatalf("errors = %v, want the connection error then backoff.ErrFirstDelayNotPositive", errs)
	}
}

func TestClientReconnect_LongestBelowFirstIsReported(t *testing.T) {
	getInstances := setupMockEventSource()
	recordDelays()
	client := New(&Config{}).Client(&ClientConfig{Endpoint: "/events", RetryInterval: 100, MaxRetryDelay: 10})
	var errs []error
	client.OnError(func(err error) { errs = append(errs, err) })
	client.Connect()

	failOnce(getInstances()[0])

	if len(errs) != 2 || errs[1] != backoff.ErrLongestBelowFirst {
		t.Fatalf("errors = %v, want the connection error then backoff.ErrLongestBelowFirst", errs)
	}
}
```
The existing `TestClientOnEvent_SurvivesReconnect` (10 / 50 ms) must keep passing unchanged.

## Stage 3 — the one style failure of the suite (`tests/test_router_test.go:18`)

`gotest` reports `tests/test_router_test.go:18:5: could remove embedded field "Router" from selector`. Change ONLY line 18: `tr.Router.Get(path, func(ctx router.Context) {` → `tr.Get(path, func(ctx router.Context) {`. **Do NOT touch line 17** (`tr.Router.Stream(...)`): `testRouter` overrides `Stream`, so removing the selector there would call itself forever.

## Stage 4 — documentation

`docs/CONFIG.md` (and the `ClientConfig` snippet of `docs/USAGE.md` around line 113 if it describes the retry fields): state that the wait doubles after each failed attempt from `RetryInterval` up to `MaxRetryDelay` (milliseconds, `RetryInterval` > 0 and `MaxRetryDelay` ≥ `RetryInterval`), that the policy is `webtyp.com/backoff`, and that an invalid pair is reported through `OnError` and not retried. Do not add a `docs/diagrams` file. Permanent docs never link `docs/PLAN.md`.

## Delivery check

1. `gotest` ends with vet ✅, style ✅, race ✅, tests ✅, wasm ✅ (before this plan only style failed, on the line of Stage 3). A tree that does not compile or whose tests did not run is a failed delivery.
2. `grep -n "1 << c.reconnectAttempts\|Default 1s" *.go` → empty; `grep -n "webtyp.com/backoff" go.mod client.go` shows both.
3. `git diff --stat main` lists `client.go`, `client_config.go`, `go.mod`, `go.sum`, `tests/reconnect_test.go`, `tests/test_router_test.go`, `docs/CONFIG.md`, and the already-pending `AGENTS.md`, `definition.go`, `definition_gen.go`; nothing is deleted except the old names `models.go` / `models_orm.go` (renamed, not removed).
