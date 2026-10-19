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

// subscription is the handle Subscribe returns. SSEClient has no unsubscribe, so Cancel only stops delivery: the
// EventSource listener stays attached and ignores the message.
type subscription struct{ cancelled bool }

func (s *subscription) Cancel() { s.cancelled = true }

func (s Subscriber) Subscribe(topic string, h events.Handler) events.Subscription {
	sub := &subscription{}
	s.Client.OnEvent(topic, func(*SSEMessage) {
		if sub.cancelled {
			return
		}
		h(events.Event{Topic: topic})
	})
	return sub
}

var _ events.Subscriber = Subscriber{}
