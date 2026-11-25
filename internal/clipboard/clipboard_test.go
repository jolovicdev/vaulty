package clipboard

import (
	"sync"
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

// TestStaleTimerDoesNotTouchNewerCopy covers a clear timer whose callback
// already fired as a newer copy arrived: it must neither wipe the newer
// value nor forget it so its own timer fails to clear it.
func TestStaleTimerDoesNotTouchNewerCopy(t *testing.T) {
	var (
		mu       sync.Mutex
		contents string
	)
	readStarted := make(chan struct{}, 1)
	releaseRead := make(chan struct{})

	m := &Manager{
		readFn: func() (string, error) {
			mu.Lock()
			v := contents
			mu.Unlock()
			select {
			case readStarted <- struct{}{}:
				<-releaseRead
			default:
			}
			return v, nil
		},
		writeFn: func(s string) error {
			mu.Lock()
			contents = s
			mu.Unlock()
			return nil
		},
	}

	if err := m.Copy("secret-A", 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	<-readStarted

	copyDone := make(chan error, 1)
	go func() {
		copyDone <- m.Copy("secret-B", 80*time.Millisecond)
	}()
	// Give Copy("secret-B") a moment to queue behind the in-flight clear.
	time.Sleep(15 * time.Millisecond)
	close(releaseRead)

	if err := <-copyDone; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	got := contents
	mu.Unlock()
	if got != "secret-B" {
		t.Fatalf("clipboard = %q right after Copy(B), want secret-B", got)
	}
	if m.Expires().IsZero() {
		t.Fatal("Expires was cleared by the stale timer")
	}

	// Secret B's own timer must still fire and wipe it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		v := contents
		mu.Unlock()
		if v == "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("secret-B was never cleared after its own timeout")
}

// TestStaleGenerationIgnoredAfterNewerCopy covers a clear callback that
// fired just before Copy(B) ran and then acquired ioMu afterwards.
func TestStaleGenerationIgnoredAfterNewerCopy(t *testing.T) {
	var contents string
	m := &Manager{
		readFn:  func() (string, error) { return contents, nil },
		writeFn: func(s string) error { contents = s; return nil },
	}
	if err := m.Copy("secret-A", 0); err != nil {
		t.Fatal(err)
	}
	oldGen := m.gen
	if err := m.Copy("secret-B", time.Minute); err != nil {
		t.Fatal(err)
	}

	m.clearIfOurs(oldGen)
	if contents != "secret-B" {
		t.Fatalf("stale clear wiped clipboard to %q", contents)
	}
	if m.Expires().IsZero() {
		t.Fatal("stale clear wiped Expires for the newer copy")
	}
	m.ClearNow()
}
