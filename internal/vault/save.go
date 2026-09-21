package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tobischo/gokeepasslib/v3"
)

// BackupCount is how many rolling backups are kept beside the vault.
const BackupCount = 5

const backupSuffix = ".bak"

// DiskState reports whether the file on disk still matches what we loaded.
type DiskState struct {
	Changed bool      `json:"changed"`
	Missing bool      `json:"missing"`
	ModTime time.Time `json:"modTime"`
}

// CheckDisk compares the vault file against the fingerprint taken when it was
// read. A different size or modification time means another writer, most
// likely a sync client, got there first.
func (v *Vault) CheckDisk() (DiskState, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.checkDiskLocked()
}

func (v *Vault) checkDiskLocked() (DiskState, error) {
	now, err := stat(v.path)
	if os.IsNotExist(err) {
		return DiskState{Changed: true, Missing: true}, nil
	}
	if err != nil {
		return DiskState{}, err
	}
	changed := now.Size != v.loaded.Size || !now.ModTime.Equal(v.loaded.ModTime)
	return DiskState{Changed: changed, ModTime: now.ModTime}, nil
}

// Save writes the database back to its own path. It refuses to run when the
// file changed underneath us; resolve that with Reload, SaveAs or Merge. The
// first save of a session copies the file it replaces into the rolling
// backups.
func (v *Vault) Save() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return err
	}

	if _, err := os.Stat(v.path); err == nil {
		state, err := v.checkDiskLocked()
		if err != nil {
			return err
		}
		if state.Changed {
			return ErrDiskChanged
		}
		if !v.backedUp {
			if err := v.rotateBackups(); err != nil {
				return err
			}
			v.backedUp = true
		}
	}

	if err := v.writeTo(v.path); err != nil {
		return err
	}

	stamp, err := stat(v.path)
	if err != nil {
		return err
	}
	v.loaded = stamp
	v.dirty = false
	return nil
}

// SaveAs writes the database to a new path and adopts it. Used when the user
// resolves a disk conflict by keeping both files.
func (v *Vault) SaveAs(path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return err
	}
	if err := v.writeTo(path); err != nil {
		return err
	}
	stamp, err := stat(path)
	if err != nil {
		return err
	}
	v.path = path
	v.loaded = stamp
	v.dirty = false
	return nil
}

// writeTo encrypts the database into a temporary file in the destination
// directory, flushes it to the platter, then renames it over the target. A
// rename within one filesystem is atomic, so a crash leaves either the old
// file or the new one, never a half-written vault.
func (v *Vault) writeTo(path string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vaulty-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Both run only on the error paths below, where the temp file is
		// already being abandoned; there is nothing to do with a failure.
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return err
	}

	// Encode expects protected values locked; it unlocks and relocks them
	// itself while writing. Lock here so that pairing balances, then unlock
	// again afterwards so the session can keep reading.
	if err := v.db.LockProtectedEntries(); err != nil {
		return err
	}
	encodeErr := gokeepasslib.NewEncoder(tmp).Encode(v.db)
	if unlockErr := v.db.UnlockProtectedEntries(); unlockErr != nil && encodeErr == nil {
		encodeErr = unlockErr
	}
	if encodeErr != nil {
		return encodeErr
	}

	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDir(dir)
}

// rotateBackups copies the current file to <name>.1.bak and shifts the older
// ones down, dropping anything past BackupCount.
func (v *Vault) rotateBackups() error {
	for i := BackupCount; i >= 1; i-- {
		from := v.backupPath(i)
		if i == BackupCount {
			if err := os.Remove(from); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		to := v.backupPath(i + 1)
		if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return copyFile(v.path, v.backupPath(1))
}

func (v *Vault) backupPath(n int) string {
	return fmt.Sprintf("%s.%d%s", v.path, n, backupSuffix)
}

// Backups lists the existing rolling backups, newest first.
func (v *Vault) Backups() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := []string{}
	for i := 1; i <= BackupCount; i++ {
		p := v.backupPath(i)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// copyFile is only ever called with paths this package derived from the
// vault's own location, which is why the destination is not re-validated.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600) //nolint:gosec // dst comes from backupPath
}

// Reload discards in-memory changes and reads the file again, reusing the
// key material from the original unlock.
func (v *Vault) Reload() error {
	fresh, err := v.reopen()
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.db = fresh.db
	v.loaded = fresh.loaded
	v.dirty = false
	// The file on disk is one this session did not write, so the next save
	// backs it up before replacing it.
	v.backedUp = false
	return nil
}

// reopen reads the vault's own path again with the credentials this session
// already holds.
func (v *Vault) reopen() (*Vault, error) {
	v.mu.RLock()
	if err := v.checkOpen(); err != nil {
		v.mu.RUnlock()
		return nil, err
	}
	path, dbc := v.path, v.db.Credentials
	v.mu.RUnlock()
	return openWith(path, dbc)
}

// MergeFromDisk resolves a changed-on-disk conflict by folding the file's
// current contents into this session and adopting its fingerprint, which is
// what allows the Save that follows to proceed.
func (v *Vault) MergeFromDisk() (MergeResult, error) {
	disk, err := v.reopen()
	if err != nil {
		return MergeResult{}, err
	}
	defer disk.Close()

	res, err := v.Merge(disk)
	if err != nil {
		return res, err
	}

	v.mu.Lock()
	v.loaded = disk.loaded
	// As in Reload: the save that follows replaces another writer's file.
	v.backedUp = false
	v.mu.Unlock()
	return res, nil
}

// ConflictCopyPath suggests a filename beside the vault for a "save a copy"
// resolution, e.g. secrets.kdbx -> secrets.conflict-20260921-113000.kdbx.
func (v *Vault) ConflictCopyPath(now time.Time) string {
	p := v.Path()
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	return fmt.Sprintf("%s.conflict-%s%s", base, now.Format("20060102-150405"), ext)
}
