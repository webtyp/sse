package sse_test

import (
	. "webtyp.com/sse"
	"testing"
)

// Common test helpers and data

// testLog is a simple logger for testing
func testLog(t *testing.T) func(args ...any) {
	return func(args ...any) {
		t.Log(args...)
	}
}

// verifyMessage checks if a message matches expected values
func verifyMessage(t *testing.T, msg *SSEMessage, expectedEvent string, expectedData []byte) {
	if msg.Event != expectedEvent {
		t.Errorf("expected event %q, got %q", expectedEvent, msg.Event)
	}
	if string(msg.Data) != string(expectedData) {
		t.Errorf("expected data %q, got %q", expectedData, msg.Data)
	}
}
