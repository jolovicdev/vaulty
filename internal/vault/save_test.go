package vault

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSaveIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	dir := filepath.Dir(path)

	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	if _, err := v.Add(Draft{Title: "Temp Check", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	if v.Dirty() {
		t.Error("Dirty = true after Save")
	}
}

func TestSaveKeepsRollingBackups(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	creds := Credentials{Password: fixturePassword}

	// More sessions than the backup limit, so the oldest must be dropped.
	// Each saves twice, and only its first save may rotate: every edit
	// saves, so a backup per save would cycle all of them within five edits.
	var got []string
	for i := 0; i < BackupCount+2; i++ {
		v, err := Open(path, creds)
		if err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 2; j++ {
			if _, err := v.Add(Draft{Title: "Entry", Password: "p"}); err != nil {
				t.Fatal(err)
			}
			if err := v.Save(); err != nil {
				t.Fatalf("session %d save %d: %v", i, j, err)
			}
		}
		got = v.Backups()
		v.Close()
		if want := min(i+1, BackupCount); len(got) != want {
			t.Fatalf("session %d left %d backups, want %d: %v", i, len(got), want, got)
		}
	}

	all, err := sortedBackups(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != BackupCount {
		t.Errorf("found %d backup files on disk, want %d: %v", len(all), BackupCount, all)
	}

	// The newest backup must open with the same credentials, which is the
	// only property that makes a backup worth keeping.
	b, err := Open(got[0], Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatalf("open newest backup: %v", err)
	}
	b.Close()
}

// TestSaveBacksUpTheFileAMergeReplaces covers a session that has already
// made its backup and then merges another writer's version. That version is
// not one this session wrote, so it is backed up before the save replaces it.
func TestSaveBacksUpTheFileAMergeReplaces(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	creds := Credentials{Password: fixturePassword}

	ours, err := Open(path, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer ours.Close()
	if _, err := ours.Add(Draft{Title: "Ours First", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := ours.Save(); err != nil {
		t.Fatal(err)
	}

	other, err := Open(path, creds)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Add(Draft{Title: "Theirs", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}
	other.Close()
	advanceModTime(t, path)

	if _, err := ours.Add(Draft{Title: "Ours Second", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ours.MergeFromDisk(); err != nil {
		t.Fatal(err)
	}
	if err := ours.Save(); err != nil {
		t.Fatal(err)
	}

	newest, err := Open(ours.Backups()[0], creds)
	if err != nil {
		t.Fatal(err)
	}
	defer newest.Close()
	list, err := newest.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if m.Title == "Theirs" {
			found = true
		}
	}
	if !found {
		t.Error("the newest backup is not the file the merge replaced")
	}
}

func TestSaveRefusesWhenFileChangedOnDisk(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))

	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	advanceModTime(t, path)

	state, err := v.CheckDisk()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Changed {
		t.Fatal("CheckDisk reported no change after the mod time moved")
	}

	if _, err := v.Add(Draft{Title: "Should Not Land", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(); !errors.Is(err, ErrDiskChanged) {
		t.Fatalf("Save = %v, want ErrDiskChanged", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("Save overwrote the file it was told not to touch")
	}
}

func TestSaveAsAdoptsNewPath(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	copyPath := v.ConflictCopyPath(time.Date(2026, 9, 21, 11, 30, 0, 0, time.UTC))
	if !strings.HasSuffix(copyPath, ".conflict-20260921-113000.kdbx") {
		t.Fatalf("ConflictCopyPath = %q", copyPath)
	}

	if _, err := v.Add(Draft{Title: "In The Copy", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := v.SaveAs(copyPath); err != nil {
		t.Fatal(err)
	}
	if v.Path() != copyPath {
		t.Errorf("Path = %q, want %q", v.Path(), copyPath)
	}

	again, err := Open(copyPath, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if _, err := again.Detail(idOf(t, again, "In The Copy")); err != nil {
		t.Errorf("entry missing from the copy: %v", err)
	}
}

func TestReloadDiscardsChanges(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	creds := Credentials{Password: fixturePassword}

	v, err := Open(path, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	if _, err := v.Add(Draft{Title: "Discarded", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := v.Reload(); err != nil {
		t.Fatal(err)
	}
	if v.Dirty() {
		t.Error("Dirty = true after Reload")
	}

	list, err := v.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		if m.Title == "Discarded" {
			t.Error("Reload kept an unsaved entry")
		}
	}
}

func TestSaveDetectsMissingFile(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	state, err := v.CheckDisk()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Missing || !state.Changed {
		t.Fatalf("CheckDisk = %+v, want Missing and Changed", state)
	}

	// A vanished file is not a conflict: writing it back is the only sane
	// recovery, so Save must succeed.
	if err := v.Save(); err != nil {
		t.Fatalf("Save after the file vanished: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not recreated: %v", err)
	}
}

func TestSavedFilePermissionsAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows has no POSIX mode bits; Chmod there only toggles the
		// read-only flag, and a writable file reports 0666. Access is an ACL
		// question on that platform, not a mode question.
		t.Skip("POSIX permissions do not apply on Windows")
	}
	path := filepath.Join(t.TempDir(), "perm.kdbx")
	v, err := Create(path, Credentials{Password: "master"})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	fi := mustStat(t, path)
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %04o, want 0600", perm)
	}
}

// TestSyncDirWorksOnThisPlatform pins the platform split behind writeTo.
// Flushing a directory is a POSIX idiom, and on Windows the same call fails
// with "Access is denied". Running this everywhere means a platform that
// cannot do it is caught by the suite rather than by a user losing a save.
func TestSyncDirWorksOnThisPlatform(t *testing.T) {
	if err := syncDir(t.TempDir()); err != nil {
		t.Errorf("syncDir on a real directory: %v", err)
	}
}

// sortedBackups lists the rotation files so a test can assert their count
// deterministically.
func sortedBackups(dir, name string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, name+".*"+backupSuffix))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}
