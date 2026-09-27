//go:build wasm

package sse

import "webtyp.com/events"

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
