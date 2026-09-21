// Package settings persists user preferences as JSON in the OS config
// directory. It never stores a password, a key file's contents, or anything
// derived from them; the recent-vault list holds paths only.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Theme selects the colour scheme.
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeDark   Theme = "dark"
	ThemeLight  Theme = "light"
)

// Bounds on the timeouts, so a bad config file cannot disable locking
// outright or make the app unusable.
const (
	MinIdleLock     = 30 * time.Second
	MaxIdleLock     = 8 * time.Hour
	MinClipboard    = time.Second
	MaxClipboard    = 5 * time.Minute
	MaxRecentVaults = 8
)

// Recent is one previously opened vault.
type Recent struct {
	Path     string    `json:"path"`
	KeyFile  string    `json:"keyFile,omitempty"`
	LastUsed time.Time `json:"lastUsed"`
}

// Settings is the whole config file.
type Settings struct {
	Theme                Theme    `json:"theme"`
	IdleLockSeconds      int      `json:"idleLockSeconds"`
	ClipboardSeconds     int      `json:"clipboardSeconds"`
	ShowPasswordStrength bool     `json:"showPasswordStrength"`
	Recent               []Recent `json:"recent"`
}

// Default is the configuration a fresh install runs with.
func Default() Settings {
	return Settings{
		Theme:                ThemeSystem,
		IdleLockSeconds:      300,
		ClipboardSeconds:     15,
		ShowPasswordStrength: true,
		Recent:               []Recent{},
	}
}

// IdleLock is the idle timeout, clamped to the supported range.
func (s Settings) IdleLock() time.Duration {
	return clamp(time.Duration(s.IdleLockSeconds)*time.Second, MinIdleLock, MaxIdleLock)
}

// ClipboardClear is the clipboard timeout, clamped to the supported range.
func (s Settings) ClipboardClear() time.Duration {
	return clamp(time.Duration(s.ClipboardSeconds)*time.Second, MinClipboard, MaxClipboard)
}

func clamp(d, lo, hi time.Duration) time.Duration {
	switch {
	case d < lo:
		return lo
	case d > hi:
		return hi
	}
	return d
}

// Store reads and writes the config file.
type Store struct {
	path string
}

// NewStore places the config at <os config dir>/vaulty/settings.json.
func NewStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "vaulty", "settings.json")}, nil
}

// NewStoreAt is used by tests to point at a temporary directory.
func NewStoreAt(path string) *Store { return &Store{path: path} }

// Path is the config file location, shown in the settings screen.
func (s *Store) Path() string { return s.path }

// Load reads the config, returning defaults when the file does not exist yet.
// A corrupt file also yields defaults rather than blocking the app; the user
// can see the path and fix or delete it.
func (s *Store) Load() Settings {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Default()
	}
	out := Default()
	if err := json.Unmarshal(data, &out); err != nil {
		return Default()
	}
	if out.Theme != ThemeDark && out.Theme != ThemeLight {
		out.Theme = ThemeSystem
	}
	if out.Recent == nil {
		out.Recent = []Recent{}
	}
	return out
}

// Save writes the config atomically, so a crash mid-write cannot leave an
// unreadable file.
func (s *Store) Save(v Settings) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		// Both run only on the error paths below, where the temp file is
		// already being abandoned; there is nothing to do with a failure.
		_ = tmp.Close()
		_ = os.Remove(name)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

// ErrNoPath is returned when a recent entry is added without a path.
var ErrNoPath = errors.New("settings: empty vault path")

// Remember moves a vault to the front of the recent list, dropping the
// oldest beyond MaxRecentVaults.
func (v *Settings) Remember(path, keyFile string, now time.Time) error {
	if path == "" {
		return ErrNoPath
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	out := make([]Recent, 0, len(v.Recent)+1)
	out = append(out, Recent{Path: abs, KeyFile: keyFile, LastUsed: now})
	for _, r := range v.Recent {
		if r.Path == abs {
			continue
		}
		out = append(out, r)
		if len(out) == MaxRecentVaults {
			break
		}
	}
	v.Recent = out
	return nil
}

// Forget removes a vault from the recent list.
func (v *Settings) Forget(path string) {
	out := make([]Recent, 0, len(v.Recent))
	for _, r := range v.Recent {
		if r.Path != path {
			out = append(out, r)
		}
	}
	v.Recent = out
}

// PruneMissing drops recent entries whose file is gone.
func (v *Settings) PruneMissing() {
	out := make([]Recent, 0, len(v.Recent))
	for _, r := range v.Recent {
		if _, err := os.Stat(r.Path); err == nil {
			out = append(out, r)
		}
	}
	v.Recent = out
}
