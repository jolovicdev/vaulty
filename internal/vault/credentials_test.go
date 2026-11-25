package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tobischo/gokeepasslib/v3"
)

func TestChangeCredentialsRotatesPasswordAndKeyFile(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	keyFile := filepath.Join("testdata", "key.keyx")

	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	oldSalt := v.db.Header.FileHeaders.KdfParameters.Salt
	oldIV := append([]byte(nil), v.db.Header.FileHeaders.EncryptionIV...)

	if err := v.ChangeCredentials(Credentials{Password: "new-master", KeyFile: keyFile}); err != nil {
		t.Fatalf("ChangeCredentials: %v", err)
	}
	if oldSalt == v.db.Header.FileHeaders.KdfParameters.Salt {
		t.Error("KDF salt was not rotated")
	}
	if bytes.Equal(oldIV, v.db.Header.FileHeaders.EncryptionIV) {
		t.Error("encryption IV was not rotated")
	}

	// Old credentials must no longer unlock the file on disk.
	if _, err := Open(path, Credentials{Password: fixturePassword}); err == nil {
		t.Error("old password still unlocked the vault")
	}
	// New password + key file must unlock it cleanly.
	reopened, err := Open(path, Credentials{Password: "new-master", KeyFile: keyFile})
	if err != nil {
		t.Fatalf("reopen with new credentials: %v", err)
	}
	defer reopened.Close()
	if _, err := reopened.Detail(idOf(t, reopened, "GitHub")); err != nil {
		t.Errorf("GitHub missing after credential rotation: %v", err)
	}
}

func TestChangeCredentialsRollsBackOnDiskConflict(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	advanceModTime(t, path)

	if err := v.ChangeCredentials(Credentials{Password: "other"}); !errors.Is(err, ErrDiskChanged) {
		t.Fatalf("ChangeCredentials = %v, want ErrDiskChanged", err)
	}
	// Credentials must still match the disk file so Reload works.
	if err := v.Reload(); err != nil {
		t.Fatalf("Reload after failed ChangeCredentials: %v", err)
	}
}

func TestOpenReportsArgon2idExplicitly(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	idx := bytes.Index(raw, gokeepasslib.KdfArgon2)
	if idx < 0 {
		t.Fatal("fixture does not contain Argon2d KDF UUID")
	}
	patched := append([]byte(nil), raw...)
	copy(patched[idx:idx+len(kdfArgon2id)], kdfArgon2id)

	out := filepath.Join(t.TempDir(), "argon2id.kdbx")
	if err := os.WriteFile(out, patched, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = Open(out, Credentials{Password: fixturePassword})
	if !errors.Is(err, ErrArgon2id) {
		t.Fatalf("Open(Argon2id) = %v, want ErrArgon2id", err)
	}
}
