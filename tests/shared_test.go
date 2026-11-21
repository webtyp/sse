package sse_test

import "testing"

// Common test helpers and data

// testLog is a simple logger for testing
func testLog(t *testing.T) func(args ...any) {
	return func(args ...any) {
		t.Log(args...)
	}
}
