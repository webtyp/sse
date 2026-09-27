//go:build !wasm

package sse_test

import (
	"testing"
	"time"

	"webtyp.com/events"
	. "webtyp.com/fmt"
	"webtyp.com/model"
	. "webtyp.com/sse"
)

type fakePayload struct{ Value string }

func (p *fakePayload) IsNil() bool                      { return p == nil }
func (p *fakePayload) EncodeFields(w model.FieldWriter) { w.String("value", p.Value) }

func TestPublisher_PushesTypedEventOverSSE(t *testing.T) {
	tSSE := New(&Config{Log: testLog(t)})
	srv := tSSE.Server(&ServerConfig{
		Path:                "/events",
		Access:              model.AccessPublic,
		ClientChannelBuffer: 10,
		HistoryReplayBuffer: 10,
		ChannelProvider:     &mockChannelProvider{channels: []string{"catalog"}},
	})
	pub := Publisher{Server: srv}

	var r testRouter
	srv.MountAPI(&r)

	st := newMockStreamer("GET", "/events")
	go r.Invoke("GET", "/events", st)
	time.Sleep(50 * time.Millisecond) // let the connection register — same wait TestStreamHandlerPublishEvent uses

	pub.Publish(events.Event{Topic: "catalog", Payload: &fakePayload{Value: "hello"}})
	time.Sleep(100 * time.Millisecond)

	out := st.Output()
	if !Contains(out, "event: catalog") {
		t.Errorf("expected SSE event name %q in output, got: %s", "catalog", out)
	}
	if !Contains(out, `"value":"hello"`) {
		t.Errorf("expected encoded payload in output, got: %s", out)
	}
}

func TestPublisher_PushesPayloadlessEventOverSSE_FilteringByChannel(t *testing.T) {
	tSSE := New(&Config{Log: testLog(t)})
	srv := tSSE.Server(&ServerConfig{
		Path:                "/events",
		Access:              model.AccessPublic,
		ClientChannelBuffer: 10,
		HistoryReplayBuffer: 10,
		ChannelProvider:     &mockChannelProvider{channels: []string{"chat_room.inbox.u1"}},
	})
	pub := Publisher{Server: srv}

	var r1 testRouter
	srv.MountAPI(&r1)

	// Streamer 2: on a server with provider subscribed to a different channel
	srvUnsubscribed := tSSE.Server(&ServerConfig{
		Path:                "/events2",
		Access:              model.AccessPublic,
		ClientChannelBuffer: 10,
		HistoryReplayBuffer: 10,
		ChannelProvider:     &mockChannelProvider{channels: []string{"chat_room.inbox.u2"}},
	})

	var r2 testRouter
	srvUnsubscribed.MountAPI(&r2)

	// Streamer 1: subscribed to chat_room.inbox.u1
	stSubscribed := newMockStreamer("GET", "/events")
	go r1.Invoke("GET", "/events", stSubscribed)

	// Streamer 2: unsubscribed
	stUnsubscribed := newMockStreamer("GET", "/events2")
	go r2.Invoke("GET", "/events2", stUnsubscribed)

	time.Sleep(50 * time.Millisecond)

	// Publish payload-less event on topic "chat_room.inbox.u1"
	pub.Publish(events.Event{Topic: "chat_room.inbox.u1"})
	time.Sleep(100 * time.Millisecond)

	outSub := stSubscribed.Output()
	if !Contains(outSub, "event: chat_room.inbox.u1") {
		t.Errorf("expected event 'chat_room.inbox.u1' in output, got: %q", outSub)
	}

	outUnsub := stUnsubscribed.Output()
	if Contains(outUnsub, "chat_room.inbox.u1") {
		t.Errorf("unsubscribed client should not receive event, got: %q", outUnsub)
	}
}
