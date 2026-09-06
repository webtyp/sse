//go:build !wasm

package sse

import (
	"webtyp.com/events"
	"webtyp.com/json"
)

// Publisher adapts an *SSEServer to events.Publisher. It is a separate wrapper type — NOT a
// method named Publish directly on *SSEServer — because SSEServer already has
// Publish(data []byte, channel string): a byte-level API existing callers use today. Colliding
// the name would either break them or force events.Publisher's single-arg shape onto that call
// site. Composition, not renaming, is the fix.
type Publisher struct {
	Server *SSEServer
}

// Publish encodes e.Payload with the ecosystem's typed codec (never `any`, never a hand-rolled
// format) and pushes it as an SSE message: Topic becomes BOTH the SSE "event" name (so a browser
// listener can dispatch by type) and the channel (so only connections ChannelProvider resolved
// into that channel receive it). A nil or IsNil Payload sends an empty body — Topic alone is
// still meaningful (e.g. a "heartbeat" event with no data).
func (p Publisher) Publish(e events.Event) {
	var data []byte
	if e.Payload != nil && !e.Payload.IsNil() {
		if err := json.Encode(e.Payload, &data); err != nil {
			return // fire-and-forget: events.Publisher makes no delivery-error promise (see its doc)
		}
	}
	p.Server.PublishEvent(e.Topic, data, e.Topic)
}

var _ events.Publisher = Publisher{}
