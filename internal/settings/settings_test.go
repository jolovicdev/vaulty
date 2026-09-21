package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLoadReturnsDefaultsWhenAbsentOrCorrupt(t *testing.T) {
	cases := []struct {
		name    string
		content string
		write   bool
	}{
		{"no file", "", false},
		{"empty file", "", true},
		{"not json", "this is not json", true},
		{"wrong types", `{"idleLockSeconds":"soon"}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if c.write {
				if err := os.WriteFile(path, []byte(c.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got := NewStoreAt(path).Load()
			if got.Theme != ThemeSystem {
				t.Errorf("Theme = %q, want %q", got.Theme, ThemeSystem)
			}
			if got.IdleLockSeconds != 300 {
				t.Errorf("IdleLockSeconds = %d, want 300", got.IdleLockSeconds)
			}
			if got.ClipboardSeconds != 15 {
				t.Errorf("ClipboardSeconds = %d, want 15", got.ClipboardSeconds)
			}
			if got.Recent == nil {
				t.Error("Recent is nil, want an empty slice")
			}
		})
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	store := NewStoreAt(path)

	want := Settings{
		Theme:            ThemeDark,
		IdleLockSeconds:  600,
		ClipboardSeconds: 30,
		Recent: []Recent{
			{Path: "/home/u/a.kdbx", LastUsed: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)},
		},
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}

	got := store.Load()
	if got.Theme != ThemeDark {
		t.Errorf("Theme = %q, want dark", got.Theme)
	}
	if got.IdleLockSeconds != 600 {
		t.Errorf("IdleLockSeconds = %d, want 600", got.IdleLockSeconds)
	}
	if len(got.Recent) != 1 || got.Recent[0].Path != "/home/u/a.kdbx" {
		t.Errorf("Recent = %+v", got.Recent)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no POSIX mode bits; Chmod there only toggles the read-only
	// flag and a writable file reports 0666.
	if perm := fi.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Errorf("mode = %04o, want 0600", perm)
	}
}

// TestSaveNeverWritesSecretFields is the check for the rule that this file
// holds no credentials: every key in the JSON must be one of the known
// non-secret fields.
func TestSaveNeverWritesSecretFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Default()
	if err := s.Remember("/home/u/vault.kdbx", "/home/u/key.keyx", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := NewStoreAt(path).Save(s); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"theme": true, "idleLockSeconds": true, "clipboardSeconds": true,
		"showPasswordStrength": true, "recent": true,
	}
	for k := range raw {
		if !allowed[k] {
			t.Errorf("unexpected key %q in the settings file", k)
		}
	}

	// A recent entry may name a key file but must never carry its bytes, so
	// pin the shape of the entries too.
	var shape struct {
		Recent []map[string]json.RawMessage `json:"recent"`
	}
	if err := json.Unmarshal(data, &shape); err != nil {
		t.Fatal(err)
	}
	if len(shape.Recent) != 1 {
		t.Fatalf("recent has %d entries, want 1", len(shape.Recent))
	}
	allowedRecent := map[string]bool{"path": true, "keyFile": true, "lastUsed": true}
	for k := range shape.Recent[0] {
		if !allowedRecent[k] {
			t.Errorf("unexpected key %q in a recent entry", k)
		}
	}
}

func TestTimeoutsAreClamped(t *testing.T) {
	cases := []struct {
		name          string
		idle          int
		clip          int
		wantIdle      time.Duration
		wantClipboard time.Duration
	}{
		{"defaults", 300, 15, 5 * time.Minute, 15 * time.Second},
		{"zero becomes the floor", 0, 0, MinIdleLock, MinClipboard},
		{"negative becomes the floor", -60, -5, MinIdleLock, MinClipboard},
		{"absurd becomes the ceiling", 999999, 999999, MaxIdleLock, MaxClipboard},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Settings{IdleLockSeconds: c.idle, ClipboardSeconds: c.clip}
			if got := s.IdleLock(); got != c.wantIdle {
				t.Errorf("IdleLock = %v, want %v", got, c.wantIdle)
			}
			if got := s.ClipboardClear(); got != c.wantClipboard {
				t.Errorf("ClipboardClear = %v, want %v", got, c.wantClipboard)
			}
		})
	}
}

func TestRememberMovesToFrontAndCaps(t *testing.T) {
	s := Default()
	now := time.Now()

	for i := 0; i < MaxRecentVaults+3; i++ {
		p := filepath.Join(t.TempDir(), "v.kdbx")
		if err := s.Remember(p, "", now); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Recent) > MaxRecentVaults {
		t.Errorf("Recent has %d entries, want at most %d", len(s.Recent), MaxRecentVaults)
	}

	// Remembering an existing path moves it to the front without duplicating.
	first := s.Recent[len(s.Recent)-1].Path
	if err := s.Remember(first, "", now); err != nil {
		t.Fatal(err)
	}
	if s.Recent[0].Path != first {
		t.Errorf("Recent[0] = %q, want %q", s.Recent[0].Path, first)
	}
	seen := map[string]int{}
	for _, r := range s.Recent {
		seen[r.Path]++
		if seen[r.Path] > 1 {
			t.Errorf("duplicate entry for %q", r.Path)
		}
	}

	if err := s.Remember("", "", now); !errors.Is(err, ErrNoPath) {
		t.Errorf("Remember with an empty path = %v, want ErrNoPath", err)
	}
}

func TestForgetAndPruneMissing(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.kdbx")
	if err := os.WriteFile(real, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(dir, "gone.kdbx")

	s := Default()
	s.Recent = []Recent{{Path: real}, {Path: gone}}

	s.PruneMissing()
	if len(s.Recent) != 1 || s.Recent[0].Path != real {
		t.Fatalf("after PruneMissing: %+v", s.Recent)
	}

	s.Forget(real)
	if len(s.Recent) != 0 {
		t.Errorf("after Forget: %+v", s.Recent)
	}
}
