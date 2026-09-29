//go:build !wasm

package sse_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webtyp.com/model"
	"webtyp.com/server/httpd"
	"webtyp.com/sse"
)

func TestClientDisconnect(t *testing.T) {
	mux := http.NewServeMux()
	r := httpd.NewRouter(mux)
	serverCfg := &sse.ServerConfig{
		Path:            "/events",
		Access:          model.AccessPublic,
		ChannelProvider: &mockChannelProvider{channels: []string{"c1"}},
	}
	sseServer := sse.New(nil).Server(serverCfg)
	sseServer.MountAPI(r)

	ts := httptest.NewServer(mux)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Cancel context to simulate client disconnect
	cancel()

	// ts.Close() should complete in less than 2s. If client disconnect is not handled,
	// ts.Close() will block waiting for active streamHandler goroutine to exit.
	closed := make(chan struct{})
	go func() {
		ts.Close()
		close(closed)
	}()

	select {
	case <-closed:
		// Server closed cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("httptest.Server.Close() timed out waiting for stream handler to finish after client disconnect")
	}
}
