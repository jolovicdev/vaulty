package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jolovicdev/vaulty/internal/settings"
	"github.com/jolovicdev/vaulty/internal/vault"
)

// newTestApp builds an App with its settings in a temp dir, so a test never
// touches the real config file.
func newTestApp(t *testing.T, idleSeconds int) *App {
	t.Helper()
	store := settings.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	prefs := settings.Default()
	prefs.IdleLockSeconds = idleSeconds
	if err := store.Save(prefs); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.store = store
	a.cached = store.Load()
	return a
}

// openTestVault creates a vault the App can hold, without going through the
// Unlock binding, which would also write to the recent list. It returns the
// id of an entry that has a TOTP secret, because a binding that fails never
// reaches the idle timer and so would not exercise the polling path.
func openTestVault(t *testing.T, a *App) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "idle.kdbx")
	v, err := vault.Create(path, vault.Credentials{Password: "master"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)

	id, err := v.Add(vault.Draft{
		Title:    "Has A Code",
		Password: "p",
		TOTPSeed: "otpauth://totp/Test?secret=JBSWY3DPEHPK3PXP",
	})
	if err != nil {
		t.Fatal(err)
	}

	a.mu.Lock()
	a.v = v
	a.mu.Unlock()
	a.Touch()
	return id
}

// TestPolledBindingsDoNotDeferTheAutoLock checks that only real input puts
// off the idle lock. A selected entry refreshes its one-time code every
// period; if that counted as activity, the vault would never lock while such
// an entry was on screen.
func TestPolledBindingsDoNotDeferTheAutoLock(t *testing.T) {
	// The settings floor is 30s, so drive the timer directly rather than
	// waiting for a real idle period.
	const idle = 40 * time.Millisecond

	cases := []struct {
		name string
		// call returns the binding's error so the test can confirm the call
		// actually worked while the vault was still open. A binding that
		// fails never reaches the idle timer, and a test built on a failing
		// call would pass whatever the timer did.
		call       func(a *App, id string) error
		wantLocked bool
	}{
		{
			name:       "no call at all",
			call:       func(*App, string) error { return nil },
			wantLocked: true,
		},
		{
			name: "TOTP polling",
			call: func(a *App, id string) error {
				_, err := a.TOTP(id)
				return err
			},
			wantLocked: true,
		},
		{
			name: "Status polling",
			call: func(a *App, _ string) error {
				a.Status()
				return nil
			},
			wantLocked: true,
		},
		{
			name: "real activity",
			call: func(a *App, _ string) error {
				a.Touch()
				return nil
			},
			wantLocked: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := newTestApp(t, 300)
			id := openTestVault(t, a)

			// The call has to work while the vault is open, or the polling
			// it stands for is never actually performed below.
			if err := c.call(a, id); err != nil {
				t.Fatalf("the polled call failed, so this test would prove nothing: %v", err)
			}

			// Re-arm with the short interval the test needs.
			a.mu.Lock()
			if a.idle != nil {
				a.idle.Stop()
			}
			a.idle = time.AfterFunc(idle, a.Lock)
			a.mu.Unlock()

			// Poll across the deadline the way the frontend would.
			for i := 0; i < 3; i++ {
				time.Sleep(idle / 2)
				_ = c.call(a, id)
			}

			if got := a.Status().Unlocked; got == c.wantLocked {
				t.Errorf("Unlocked = %v after polling, want %v", got, !c.wantLocked)
			}
		})
	}
}

// TestLockDropsTheVault checks what a lock leaves behind: no vault to read
// and no path to report.
func TestLockDropsTheVault(t *testing.T) {
	a := newTestApp(t, 300)
	openTestVault(t, a)

	if !a.Status().Unlocked {
		t.Fatal("vault is not unlocked before the test starts")
	}
	a.Lock()

	got := a.Status()
	if got.Unlocked {
		t.Error("Unlocked = true after Lock")
	}
	if got.Path != "" {
		t.Errorf("Path = %q after Lock, want empty", got.Path)
	}
	if _, err := a.List(vault.ListOptions{}); err == nil {
		t.Error("List succeeded after Lock")
	}
	if _, err := a.Reveal("any", "Password"); err == nil {
		t.Error("Reveal succeeded after Lock")
	}
}

// TestBeforeCloseNeverTrapsTheWindow pins the rule that makes the window
// always closable: beforeClose blocks only while there is something to lose,
// and answering the question always lets the next close through.
//
// Getting this wrong is unrecoverable from inside the app, which is why the
// no-frontend case is covered too.
func TestBeforeCloseNeverTrapsTheWindow(t *testing.T) {
	t.Run("no vault open", func(t *testing.T) {
		a := newTestApp(t, 300)
		if a.hasUnsavedWork() {
			t.Error("reported unsaved work with no vault open")
		}
	})

	t.Run("vault open with nothing unsaved", func(t *testing.T) {
		a := newTestApp(t, 300)
		openTestVault(t, a)
		// Create wrote the file on the way out, and nothing changed since.
		if err := a.Save(); err != nil {
			t.Fatal(err)
		}
		if a.hasUnsavedWork() {
			t.Error("reported unsaved work on a clean vault")
		}
	})

	t.Run("unsaved work blocks once, then the answer lets it through", func(t *testing.T) {
		a := newTestApp(t, 300)
		openTestVault(t, a)
		// Writes autosave, so the only way work stays unsaved is an
		// autosave that was refused because the file changed underneath.
		holdAnEditBackFromDisk(t, a)
		if !a.Status().Dirty {
			t.Fatal("the vault is not dirty after a refused autosave")
		}
		if !a.hasUnsavedWork() {
			t.Fatal("the close would have gone through with unsaved work")
		}

		// Answering the question sets the flag that releases the next
		// close. runtime.Quit needs a live frontend, so the flag is set
		// here the way ConfirmQuit sets it.
		a.mu.Lock()
		a.quitting = true
		a.mu.Unlock()

		if a.hasUnsavedWork() {
			t.Error("still blocking after the question was answered")
		}
	})

	// Without a frontend there is nobody to answer the question, so
	// blocking would strand the user. This is the case the guard covers.
	t.Run("no frontend lets the close through", func(t *testing.T) {
		a := newTestApp(t, 300)
		openTestVault(t, a)
		holdAnEditBackFromDisk(t, a)
		if a.beforeClose(context.Background()) {
			t.Error("beforeClose blocked with no frontend to ask")
		}
	})
}

// holdAnEditBackFromDisk leaves the session holding a change that is not on
// disk, which since autosave is the conflict case and nothing else: another
// writer touches the file, so the save behind the next write is refused.
func holdAnEditBackFromDisk(t *testing.T, a *App) {
	t.Helper()
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	path := a.Status().Path
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	future := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddEntry(vault.Draft{Title: "Held", Password: "p"}); err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
}

// TestWritesAutosave pins the behaviour behind dropping the Save button:
// every binding that changes the vault leaves the file on disk up to date,
// so there is no window in which work exists only in memory.
func TestWritesAutosave(t *testing.T) {
	a := newTestApp(t, 300)
	id := openTestVault(t, a)

	// openTestVault adds an entry directly through the vault package, so
	// start from a known saved state.
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		write func(t *testing.T) error
	}{
		{"AddEntry", func(*testing.T) error {
			_, err := a.AddEntry(vault.Draft{Title: "Added", Password: "p"})
			return err
		}},
		{"UpdateEntry", func(*testing.T) error {
			return a.UpdateEntry(id, vault.Draft{Title: "Renamed", Password: "p2"})
		}},
		{"AddGroup", func(*testing.T) error {
			_, err := a.AddGroup("", "Servers")
			return err
		}},
		{"ImportFile", func(t *testing.T) error { return a.ImportFile(writeExport(t)) }},
		{"DeleteEntry", func(*testing.T) error { return a.DeleteEntry(id) }},
		{"EmptyRecycleBin", func(*testing.T) error { return a.EmptyRecycleBin() }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.write(t); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if a.Status().Dirty {
				t.Errorf("%s left unsaved changes behind", c.name)
			}
		})
	}
}

// TestAutosaveKeepsTheEditWhenTheFileChanged is the conflict path. The edit
// must survive in memory and the file must be left alone, because the
// alternative is losing whichever side the app decided to drop.
func TestAutosaveKeepsTheEditWhenTheFileChanged(t *testing.T) {
	a := newTestApp(t, 300)
	openTestVault(t, a)
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}

	path := a.Status().Path
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Another writer touches the file.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	future := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	// The write succeeds and the autosave behind it does not.
	if _, err := a.AddEntry(vault.Draft{Title: "Held In Memory", Password: "p"}); err != nil {
		t.Fatalf("AddEntry reported an error for a conflict it should hold: %v", err)
	}

	if !a.Status().Dirty {
		t.Error("the edit was not kept after the failed autosave")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the autosave overwrote a file that had changed on disk")
	}

	// The entry is still there to be saved once the conflict is resolved.
	list, err := a.List(vault.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if m.Title == "Held In Memory" {
			found = true
		}
	}
	if !found {
		t.Error("the edit is gone from the session as well as from the file")
	}
}

// TestBrowserURLGivesABareAddressAScheme covers entries that store an
// address the way people type one. Wails refuses a URL with no scheme and
// only logs the refusal, so without this the Open URL action does nothing.
// TestFailedImportSaveLeavesNothingBehind covers a save that fails for a
// reason other than a conflict. The import must come back out of memory, or
// the retry the dialog offers adds every entry a second time.
func TestFailedImportSaveLeavesNothingBehind(t *testing.T) {
	a := newTestApp(t, 300)
	openTestVault(t, a)
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	export := writeExport(t)

	// With its directory gone, the vault cannot be written on any platform.
	dir := filepath.Dir(a.Status().Path)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := a.ImportFile(export); err == nil {
		t.Fatal("ImportFile succeeded with nowhere to save")
	}
	if a.Status().Dirty {
		t.Error("the failed import left unsaved changes behind")
	}
	if n := importGroups(t, a); n != 0 {
		t.Fatalf("%d import groups after the failed save, want 0", n)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := a.ImportFile(export); err != nil {
		t.Fatal(err)
	}
	if n := importGroups(t, a); n != 1 {
		t.Errorf("%d import groups after the retry, want 1", n)
	}
}

// writeExport writes a one-entry Bitwarden export and returns its path.
func writeExport(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bitwarden.json")
	export := `{"encrypted": false, "items": [{"type": 1, "name": "Imported", "login": {"password": "p"}}]}`
	if err := os.WriteFile(path, []byte(export), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func importGroups(t *testing.T, a *App) int {
	t.Helper()
	groups, err := a.Groups()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, g := range groups[0].Children {
		if g.Name == importGroup("Bitwarden") {
			n++
		}
	}
	return n
}

func TestBrowserURLGivesABareAddressAScheme(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"github.com", "https://github.com"},
		{"  github.com/login ", "https://github.com/login"},
		{"nas.local:5001", "https://nas.local:5001"},
		{"https://github.com", "https://github.com"},
		{"http://intranet/login", "http://intranet/login"},
	}
	for _, c := range cases {
		if got := browserURL(c.raw); got != c.want {
			t.Errorf("browserURL(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}
