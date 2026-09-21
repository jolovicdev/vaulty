package clipboard

import (
	"testing"
	"time"
)

// TestCopyAndClearOnThisSystem exercises the real platform clipboard. It
// skips where no clipboard helper exists, which is the normal state of a
// headless build machine.
func TestCopyAndClearOnThisSystem(t *testing.T) {
	if !Available() {
		t.Skip("no clipboard backend on this system")
	}
	m := New()

	const secret = "vaulty-clipboard-test-value"
	if err := m.Copy(secret, 0); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	got, err := read()
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != secret {
		t.Fatalf("clipboard holds %q, want %q", got, secret)
	}

	m.ClearNow()
	after, err := read()
	if err != nil {
		t.Fatalf("read after clear: %v", err)
	}
	if after == secret {
		t.Error("ClearNow left our value on the clipboard")
	}
}

// TestClearLeavesSomebodyElsesValueAlone is the property the timed clear
// rests on: it must not wipe whatever the user copied in the meantime.
func TestClearLeavesSomebodyElsesValueAlone(t *testing.T) {
	if !Available() {
		t.Skip("no clipboard backend on this system")
	}
	m := New()

	if err := m.Copy("ours", 0); err != nil {
		t.Fatal(err)
	}
	// Simulate the user copying something else.
	if err := write("theirs"); err != nil {
		t.Fatal(err)
	}

	m.ClearNow()
	got, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if got != "theirs" {
		t.Errorf("clipboard holds %q, want the value we did not write", got)
	}
}

func TestExpiresReportsThePendingClear(t *testing.T) {
	if !Available() {
		t.Skip("no clipboard backend on this system")
	}
	m := New()

	if !m.Expires().IsZero() {
		t.Error("Expires is set before anything was copied")
	}
	if err := m.Copy("with-timeout", time.Minute); err != nil {
		t.Fatal(err)
	}
	if m.Expires().IsZero() {
		t.Fatal("Expires is zero after a copy with a timeout")
	}
	if d := time.Until(m.Expires()); d <= 0 || d > time.Minute {
		t.Errorf("Expires is %v away, want within the minute", d)
	}

	m.ClearNow()
	if !m.Expires().IsZero() {
		t.Error("Expires still set after ClearNow")
	}
}

func TestTimedClearFires(t *testing.T) {
	if !Available() {
		t.Skip("no clipboard backend on this system")
	}
	m := New()

	const secret = "vaulty-timed-clear"
	if err := m.Copy(secret, 150*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	// Give the timer room on a loaded machine.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, err := read()
		if err == nil && got != secret {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("the clipboard still held our value after the timeout")
}

// TestCopyWithoutTimeoutSchedulesNothing keeps a zero timeout from arming a
// clear, which is the setting for "never clear".
func TestCopyWithoutTimeoutSchedulesNothing(t *testing.T) {
	if !Available() {
		t.Skip("no clipboard backend on this system")
	}
	m := New()
	if err := m.Copy("no-timeout", 0); err != nil {
		t.Fatal(err)
	}
	if !m.Expires().IsZero() {
		t.Error("Expires is set for a copy with no timeout")
	}
	m.ClearNow()
}
