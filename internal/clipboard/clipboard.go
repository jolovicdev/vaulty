// Package clipboard writes a value to the system clipboard and clears it
// again after a timeout. Secrets are handed to this package and never
// returned; the frontend asks for a copy and gets a confirmation, not the
// value.
package clipboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"sync"
	"time"
)

// ErrUnavailable means no clipboard helper could be found on this system.
var ErrUnavailable = errors.New("clipboard: no usable clipboard backend")

// DefaultClearAfter is how long a copied secret stays on the clipboard.
const DefaultClearAfter = 15 * time.Second

// Manager owns at most one pending clear at a time.
type Manager struct {
	mu sync.Mutex
	// digest identifies the value we put there, so a clear can check the
	// clipboard still holds our value before wiping someone else's.
	digest  [32]byte
	hasCopy bool
	timer   *time.Timer
	// Expires is when the pending clear fires, for the countdown in the UI.
	expires time.Time
}

// New returns a Manager.
func New() *Manager { return &Manager{} }

// Copy places value on the clipboard and schedules a clear after d. A d of
// zero or less disables the automatic clear.
func (m *Manager) Copy(value string, d time.Duration) error {
	if err := write(value); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.digest = sha256.Sum256([]byte(value))
	m.hasCopy = true
	m.expires = time.Time{}

	if d > 0 {
		m.expires = time.Now().Add(d)
		m.timer = time.AfterFunc(d, m.clearIfOurs)
	}
	return nil
}

// Expires is when the pending clear fires, or the zero time when none is
// pending.
func (m *Manager) Expires() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.expires
}

// ClearNow wipes the clipboard immediately if it still holds our value.
func (m *Manager) ClearNow() {
	m.mu.Lock()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.mu.Unlock()
	m.clearIfOurs()
}

// clearIfOurs reads the clipboard back and only wipes it when the contents
// still hash to what we wrote. If the user copied something else in the
// meantime, theirs is left alone.
func (m *Manager) clearIfOurs() {
	m.mu.Lock()
	if !m.hasCopy {
		m.mu.Unlock()
		return
	}
	want := m.digest
	m.mu.Unlock()

	current, err := read()
	if err == nil {
		got := sha256.Sum256([]byte(current))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			m.forget()
			return
		}
	}
	// A backend that cannot read the clipboard back (some Wayland
	// compositors refuse a paste request from a non-focused client) falls
	// through and clears anyway: a stale secret on the clipboard is the
	// worse outcome.
	_ = write("")
	m.forget()
}

func (m *Manager) forget() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.digest = [32]byte{}
	m.hasCopy = false
	m.expires = time.Time{}
	m.timer = nil
}
