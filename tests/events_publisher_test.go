//go:build !wasm

package sse_test

import (
	. "webtyp.com/sse"
	"testing"
	"time"

	"webtyp.com/events"
	. "webtyp.com/fmt"
	"webtyp.com/model"
)

type fakePayload struct{ Value string }

func (p *fakePayload) IsNil() bool                      { return p == nil }
func (p *fakePayload) EncodeFields(w model.FieldWriter) { w.String("value", p.Value) }

func TestPublisher_PushesTypedEventOverSSE(t *testing.T) {
	tSSE := New(&Config{Log: testLog(t)})
	srv := tSSE.Server(&ServerConfig{
		ClientChannelBuffer: 10,
		HistoryReplayBuffer: 10,
		ChannelProvider:     &mockChannelProvider{channels: []string{"catalog"}},
	})
	pub := Publisher{Server: srv}

	st := newMockStreamer()
	go srv.StreamHandler()(st)
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
