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
