//go:build !wasm

package sse_test

import (
	"sync"
	"testing"
	"time"

	. "webtyp.com/fmt"
	"webtyp.com/model"
	. "webtyp.com/sse"
)

func TestMountAPIContractCompiles(t *testing.T) {
	cfg := &Config{Log: testLog(t)}
	tSSE := New(cfg)
	server := tSSE.Server(&ServerConfig{
		Path:            "/events",
		Access:          model.AccessPublic,
		ChannelProvider: &mockChannelProvider{channels: []string{"c"}},
	})

	var r testRouter
	server.MountAPI(&r)
}

func TestStreamHandlerNoChannelProvider(t *testing.T) {
	cfg := &Config{}
	tSSE := New(cfg)
	server := tSSE.Server(&ServerConfig{
		Path:   "/events",
		Access: model.AccessPublic,
	}) // sin ChannelProvider

	var r testRouter
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("expected panic on MountAPI with nil ChannelProvider")
		}
	}()
	server.MountAPI(&r)
}

func TestStreamHandlerChannelProviderError(t *testing.T) {
	cfg := &Config{}
	tSSE := New(cfg)
	provider := &mockChannelProvider{err: Err("auth failed")}
	server := tSSE.Server(&ServerConfig{
		Path:            "/events",
		Access:          model.AccessPublic,
		ChannelProvider: provider,
	})

	var r testRouter
	server.MountAPI(&r)

	st := newMockStreamer("GET", "/events")
	r.Invoke("GET", "/events", st)

	if st.ResponseStatus() != 401 {
		t.Errorf("expected status 401, got %d", st.ResponseStatus())
	}
}

func TestStreamHandlerPublishEvent(t *testing.T) {
	cfg := &Config{Log: testLog(t)}
	tSSE := New(cfg)

	provider := &mockChannelProvider{channels: []string{"test-channel"}}
	server := tSSE.Server(&ServerConfig{
		Path:                "/events",
		Access:              model.AccessPublic,
		ClientChannelBuffer: 10,
		HistoryReplayBuffer: 10,
		ChannelProvider:     provider,
	})

	var r testRouter
	server.MountAPI(&r)

	st := newMockStreamer("GET", "/events")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Invoke("GET", "/events", st)
	}()

	// Esperar a que el handler se registre y flushee los headers
	time.Sleep(50 * time.Millisecond)

	// Publicar un evento
	server.PublishEvent("greeting", []byte("hello world"), "test-channel")

	// Dar tiempo a que el hub lo encole y el handler lo escriba
	time.Sleep(100 * time.Millisecond)

	// Verificar que el evento fue recibido antes de cortar
	output := st.Output()
	t.Logf("Output so far: %q", output)

	if !Contains(output, "event: greeting") {
		t.Error("missing event type")
	}
	if !Contains(output, "data: hello world") {
		t.Error("missing data")
	}
	if !Contains(output, "id: ") {
		t.Error("missing id")
	}
	if st.FlushCount() < 2 { // 1 header flush + ≥1 message flush
		t.Errorf("expected at least 2 flushes, got %d", st.FlushCount())
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
	}
}

func TestStreamHandlerHistoryReplay(t *testing.T) {
	cfg := &Config{Log: testLog(t)}
	tSSE := New(cfg)

	provider := &mockChannelProvider{channels: []string{"all"}}
	server := tSSE.Server(&ServerConfig{
		Path:                "/events",
		Access:              model.AccessPublic,
		ClientChannelBuffer: 10,
		HistoryReplayBuffer: 5,
		ReplayAllOnConnect:  true,
		ChannelProvider:     provider,
	})

	var r testRouter
	server.MountAPI(&r)

	// Publicar antes de conectar
	server.Publish([]byte("msg1"), "all")
	server.Publish([]byte("msg2"), "all")
	server.Publish([]byte("msg3"), "all")

	time.Sleep(30 * time.Millisecond)

	// Conectar con Last-Event-ID = "1" → debe recibir msg2 y msg3
	st := newMockStreamer("GET", "/events")
	st.SetHeader("Last-Event-ID", "1") // simula request header de entrada

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Invoke("GET", "/events", st)
	}()

	time.Sleep(100 * time.Millisecond)

	output := st.Output()
	t.Logf("History replay output: %q", output)

	if Contains(output, "data: msg1") {
		t.Error("should not receive msg1")
	}
	if !Contains(output, "data: msg2") {
		t.Error("missing msg2")
	}
	if !Contains(output, "data: msg3") {
		t.Error("missing msg3")
	}

	_ = &wg
}
