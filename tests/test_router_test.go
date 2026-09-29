//go:build !wasm

package sse_test

import (
	"sync"

	"webtyp.com/router"
	routermock "webtyp.com/router/mock"
)

type testRouter struct {
	routermock.Router
}

func (tr *testRouter) Stream(path string, h router.StreamFunc) router.Route {
	route := tr.Router.Stream(path, h)
	tr.Router.Get(path, func(ctx router.Context) {
		if st, ok := ctx.(router.Streamer); ok {
			h(st)
		}
	})
	return route
}

func (tr *testRouter) Routes() []router.RouteInfo {
	routes := tr.Router.Routes()
	seen := make(map[string]bool)
	out := make([]router.RouteInfo, 0, len(routes))
	for _, r := range routes {
		key := r.Method + ":" + r.Path
		if !seen[key] {
			seen[key] = true
			out = append(out, r)
		}
	}
	return out
}

// mockChannelProvider implements ChannelProvider for testing.
type mockChannelProvider struct {
	channels []string
	err      error
}

func (m *mockChannelProvider) ResolveChannels(_ router.Context) ([]string, error) {
	return m.channels, m.err
}

// mockStreamer implements router.Streamer with thread-safe Status reading.
type mockStreamer struct {
	routermock.Context
	mu         sync.Mutex
	flushCount int
	done       chan struct{}
}

func newMockStreamer(method, path string) *mockStreamer {
	return &mockStreamer{
		Context: routermock.Context{
			InMethod: method,
			InPath:   path,
		},
		done: make(chan struct{}),
	}
}

func (m *mockStreamer) WriteStatus(code int) {
	m.mu.Lock()
	m.Context.WriteStatus(code)
	m.mu.Unlock()
}

func (m *mockStreamer) ResponseStatus() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Status
}

func (m *mockStreamer) Flush() {
	m.mu.Lock()
	m.flushCount++
	m.mu.Unlock()
}

func (m *mockStreamer) FlushCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flushCount
}

func (m *mockStreamer) Output() string {
	return string(m.ResponseBody())
}

func (m *mockStreamer) Done() <-chan struct{} {
	return m.done
}

var _ router.Streamer = (*mockStreamer)(nil)
