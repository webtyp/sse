//go:build !wasm

package sse_test

import (
	"testing"
	"time"

	"webtyp.com/model"
	"webtyp.com/router"
	. "webtyp.com/sse"
)

func TestMountAPI_AccessPublic(t *testing.T) {
	tSSE := New(&Config{Log: testLog(t)})
	srv := tSSE.Server(&ServerConfig{
		Path:            "/events",
		Access:          model.AccessPublic,
		ChannelProvider: &mockChannelProvider{channels: []string{"all"}},
	})

	var r testRouter
	srv.MountAPI(&r)

	routes := r.Routes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	route := routes[0]
	if route.Path != "/events" {
		t.Errorf("expected path /events, got %s", route.Path)
	}
	if route.Access != model.AccessPublic {
		t.Errorf("expected AccessPublic, got %v", route.Access)
	}
}

func TestMountAPI_AccessAuthenticated(t *testing.T) {
	tSSE := New(&Config{Log: testLog(t)})
	provider := &mockChannelProvider{channels: []string{"user:1"}}
	srv := tSSE.Server(&ServerConfig{
		Path:            "/stream",
		Access:          model.AccessAuthenticated,
		ChannelProvider: provider,
	})

	var r testRouter
	srv.MountAPI(&r)

	routes := r.Routes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Access != model.AccessAuthenticated {
		t.Errorf("expected AccessAuthenticated, got %v", routes[0].Access)
	}

	// Invoke with empty UserID -> rejected by mock gate (403 Forbidden)
	ctxUnauth := newMockStreamer("GET", "/stream")
	r.Invoke("GET", "/stream", ctxUnauth)
	if ctxUnauth.ResponseStatus() != 403 {
		t.Errorf("expected status 403 for unauthenticated invoke, got %d", ctxUnauth.ResponseStatus())
	}

	// Invoke with non-empty UserID -> reaches channel provider
	ctxAuth := newMockStreamer("GET", "/stream")
	ctxAuth.SetUserID("usr_123")
	go r.Invoke("GET", "/stream", ctxAuth)
	time.Sleep(50 * time.Millisecond)

	if ctxAuth.ResponseStatus() != 200 {
		t.Errorf("expected status 200 for authenticated invoke, got %d", ctxAuth.ResponseStatus())
	}
}

func TestMountAPI_AccessGuarded(t *testing.T) {
	tSSE := New(&Config{Log: testLog(t)})
	srv := tSSE.Server(&ServerConfig{
		Path:            "/guarded-events",
		Access:          model.AccessGuarded,
		Resource:        "events",
		ChannelProvider: &mockChannelProvider{channels: []string{"guarded"}},
	})

	var r testRouter
	srv.MountAPI(&r)

	routes := r.Routes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	route := routes[0]
	if route.Access != model.AccessGuarded {
		t.Errorf("expected AccessGuarded, got %v", route.Access)
	}
	if route.Resource != "events" {
		t.Errorf("expected Resource 'events', got %q", route.Resource)
	}
	if route.Action != model.Read {
		t.Errorf("expected Action Read, got %v", route.Action)
	}
}

func TestMountAPI_Panics(t *testing.T) {
	tSSE := New(&Config{Log: testLog(t)})
	provider := &mockChannelProvider{channels: []string{"all"}}

	tests := []struct {
		name        string
		cfg         *ServerConfig
		expectedErr string
	}{
		{
			name: "missing path",
			cfg: &ServerConfig{
				Path:            "",
				ChannelProvider: provider,
			},
			expectedErr: "sse: ServerConfig.Path is required",
		},
		{
			name: "missing channel provider",
			cfg: &ServerConfig{
				Path:            "/events",
				ChannelProvider: nil,
			},
			expectedErr: "sse: ServerConfig.ChannelProvider is required",
		},
		{
			name: "guarded missing resource",
			cfg: &ServerConfig{
				Path:            "/events",
				Access:          model.AccessGuarded,
				Resource:        "",
				ChannelProvider: provider,
			},
			expectedErr: "sse: ServerConfig.Resource is required when Access is AccessGuarded",
		},
		{
			name: "public with resource",
			cfg: &ServerConfig{
				Path:            "/events",
				Access:          model.AccessPublic,
				Resource:        "events",
				ChannelProvider: provider,
			},
			expectedErr: "sse: ServerConfig.Resource must be empty unless Access is AccessGuarded",
		},
		{
			name: "authenticated with resource",
			cfg: &ServerConfig{
				Path:            "/events",
				Access:          model.AccessAuthenticated,
				Resource:        "events",
				ChannelProvider: provider,
			},
			expectedErr: "sse: ServerConfig.Resource must be empty unless Access is AccessGuarded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r testRouter
			srv := tSSE.Server(tt.cfg)

			defer func() {
				rec := recover()
				if rec == nil {
					t.Fatalf("expected panic, got none")
				}
				errStr, ok := rec.(string)
				if !ok {
					t.Fatalf("expected string panic, got %v", rec)
				}
				if errStr != tt.expectedErr {
					t.Errorf("expected panic %q, got %q", tt.expectedErr, errStr)
				}
			}()

			srv.MountAPI(&r)
		})
	}
}

func TestAPIModuleInterfaceCompliance(t *testing.T) {
	tSSE := New(&Config{})
	srv := tSSE.Server(&ServerConfig{})
	var _ router.APIModule = srv

	if srv.ModelName() != "sse" {
		t.Errorf("expected ModelName 'sse', got %q", srv.ModelName())
	}
}
