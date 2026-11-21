//go:build !wasm

package sse_test

import (
	"testing"
	"time"

	. "webtyp.com/fmt"
	. "webtyp.com/sse"
)

const agendaChanged = "clinical_encounter.agenda.changed"

// A notice reaches the browsers of the user it names and nobody else's: the doctor sees "your agenda changed", the
// receptionist connected to the same stream does not.
func TestUserNotices_ReachOnlyTheNamedUser(t *testing.T) {
	notices := NewUserNotices("/notices")
	var r testRouter
	notices.MountAPI(&r)

	doctor := newMockStreamer("GET", "/notices")
	doctor.SetUserID("doctor-1")
	other := newMockStreamer("GET", "/notices")
	other.SetUserID("reception-1")
	go r.Invoke("GET", "/notices", doctor)
	go r.Invoke("GET", "/notices", other)
	time.Sleep(50 * time.Millisecond)

	notices.Notify("doctor-1", agendaChanged)
	time.Sleep(100 * time.Millisecond)

	if !Contains(doctor.Output(), "event: "+agendaChanged) {
		t.Errorf("doctor did not receive the notice: %q", doctor.Output())
	}
	if Contains(other.Output(), agendaChanged) {
		t.Errorf("another user received the doctor's notice: %q", other.Output())
	}
}

// A connection without a signed-in user is refused: a notice is always for one person.
func TestUserNotices_RefuseAConnectionWithoutUser(t *testing.T) {
	notices := NewUserNotices("/notices")
	if _, err := notices.ResolveChannels(newMockStreamer("GET", "/notices")); err != ErrNoSessionUser {
		t.Fatalf("err = %v, want ErrNoSessionUser", err)
	}
}
