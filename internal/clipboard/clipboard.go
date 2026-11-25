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
	// ioMu serialises OS clipboard reads and writes so a timed clear cannot
	// read an older value, pause while a new copy writes, and then wipe the
	// new copy.
	ioMu sync.Mutex

	mu sync.Mutex
	// gen advances on every Copy so an already-fired timer from a previous
	// copy cannot wipe or forget the current one.
	gen uint64
	// digest identifies the value we put there, so a clear can check the
	// clipboard still holds our value before wiping someone else's.
	digest  [32]byte
	hasCopy bool
	timer   *time.Timer
	// expires is when the pending clear fires, for the countdown in the UI.
	expires time.Time

	readFn  func() (string, error)
	writeFn func(string) error
}

// New returns a Manager backed by the OS clipboard.
func New() *Manager { return &Manager{readFn: read, writeFn: write} }

// Copy places value on the clipboard and schedules a clear after d. A d of
// zero or less disables the automatic clear.
func (m *Manager) Copy(value string, d time.Duration) error {
	m.ioMu.Lock()
	defer m.ioMu.Unlock()

	if err := m.writeFn(value); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.gen++
	gen := m.gen
	m.digest = sha256.Sum256([]byte(value))
	m.hasCopy = true
	m.expires = time.Time{}

	if d > 0 {
		m.expires = time.Now().Add(d)
		m.timer = time.AfterFunc(d, func() { m.clearIfOurs(gen) })
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
	gen := m.gen
	m.mu.Unlock()
	m.clearIfOurs(gen)
}

// clearIfOurs reads the clipboard back and only wipes it when the contents
// still hash to what we wrote. If the user copied something else in the
// meantime, theirs is left alone.
func (m *Manager) clearIfOurs(gen uint64) {
	m.ioMu.Lock()
	defer m.ioMu.Unlock()

	m.mu.Lock()
	if !m.hasCopy || m.gen != gen {
		m.mu.Unlock()
		return
	}
	want := m.digest
	m.mu.Unlock()

	current, err := m.readFn()
	if err == nil {
		got := sha256.Sum256([]byte(current))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			m.forget(gen)
			return
		}
	}
	// A backend that cannot read the clipboard back (some Wayland
	// compositors refuse a paste request from a non-focused client) falls
	// through and clears anyway: a stale secret on the clipboard is the
	// worse outcome.
	_ = m.writeFn("")
	m.forget(gen)
}

func (m *Manager) forget(gen uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gen != gen {
		return
	}
	m.digest = [32]byte{}
	m.hasCopy = false
	m.expires = time.Time{}
	m.timer = nil
}
