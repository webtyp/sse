//go:build wasm

package sse_test

import (
	"testing"

	. "webtyp.com/sse"
)

// verifyMessage checks if a message matches expected values.
func verifyMessage(t *testing.T, msg *SSEMessage, expectedEvent string, expectedData []byte) {
	if msg.Event != expectedEvent {
		t.Errorf("expected event %q, got %q", expectedEvent, msg.Event)
	}
	if string(msg.Data) != string(expectedData) {
		t.Errorf("expected data %q, got %q", expectedData, msg.Data)
	}
}
