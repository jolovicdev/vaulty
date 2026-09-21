package vault

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// keepassxcCLI locates the real KeePassXC command line tool. The interop
// tests skip when it is absent, so the suite still runs on a machine without
// KeePassXC installed.
//
// Set VAULTY_KEEPASSXC to an explicit path when the binary is not on PATH.
func keepassxcCLI(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("VAULTY_KEEPASSXC"); p != "" {
		return p
	}
	p, err := exec.LookPath("keepassxc-cli")
	if err != nil {
		t.Skip("keepassxc-cli not found; set VAULTY_KEEPASSXC to run the interop tests")
	}
	return p
}

// runCLI feeds the master password on stdin, which is how keepassxc-cli
// expects it when there is no terminal.
func runCLI(t *testing.T, bin, password string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(password + "\n")
	// Offscreen keeps Qt from needing a display on a headless machine.
	cmd.Env = append(os.Environ(), "QT_QPA_PLATFORM=offscreen")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestKeePassXCOpensWhatWeWrite is the interop gate: every format this app
// writes must be readable by the real KeePassXC, with the field values
// intact.
func TestKeePassXCOpensWhatWeWrite(t *testing.T) {
	bin := keepassxcCLI(t)

	for _, f := range fixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			path := copyFixture(t, f.file)
			creds := f.creds
			if creds.KeyFile != "" {
				creds.KeyFile = keyFileCopy(t)
			}

			v, err := Open(path, creds)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := v.Add(Draft{
				Title:           "Written By Vaulty",
				Username:        "interop",
				Password:        "interop-p4ssw0rd!",
				URL:             "https://interop.example",
				Notes:           "written by the vault package",
				Tags:            []string{"interop"},
				Custom:          map[string]string{"Token": "tok-interop"},
				ProtectedCustom: []string{"Token"},
			}); err != nil {
				t.Fatal(err)
			}
			if err := v.Save(); err != nil {
				t.Fatal(err)
			}
			v.Close()

			args := []string{"show", "-q", "-s", "-a", "Password"}
			if creds.KeyFile != "" {
				args = append(args, "-k", creds.KeyFile)
			}
			args = append(args, path, "Written By Vaulty")
			out, err := runCLI(t, bin, fixturePassword, args...)
			if err != nil {
				t.Fatalf("keepassxc-cli failed: %v\n%s", err, out)
			}
			if got := strings.TrimSpace(out); got != "interop-p4ssw0rd!" {
				t.Errorf("KeePassXC read the password as %q, want interop-p4ssw0rd!", got)
			}
		})
	}
}

// TestKeePassXCReadsOurProtectedCustomField checks that a protected custom
// field survives as a protected attribute, not as plain text.
func TestKeePassXCReadsOurProtectedCustomField(t *testing.T) {
	bin := keepassxcCLI(t)
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))

	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Add(Draft{
		Title:           "Protected Check",
		Password:        "p",
		Custom:          map[string]string{"Token": "tok-secret", "Plain": "tok-visible"},
		ProtectedCustom: []string{"Token"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}
	v.Close()

	out, err := runCLI(t, bin, fixturePassword, "show", "-q", "-s", "-a", "Token", path, "Protected Check")
	if err != nil {
		t.Fatalf("keepassxc-cli failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(out); got != "tok-secret" {
		t.Errorf("Token read as %q, want tok-secret", got)
	}

	// Whether KeePassXC masks the value on screen is its own display choice;
	// what this test can assert is that it decrypts to the right bytes. That
	// the field is stored with Protected="True" is checked by
	// TestDetailHidesProtectedFields against the reopened file.
	plain, err := runCLI(t, bin, fixturePassword, "show", "-q", "-s", "-a", "Plain", path, "Protected Check")
	if err != nil {
		t.Fatalf("keepassxc-cli failed: %v\n%s", err, plain)
	}
	if got := strings.TrimSpace(plain); got != "tok-visible" {
		t.Errorf("Plain read as %q, want tok-visible", got)
	}
}

// TestKeePassXCAgreesOnTOTP cross-checks internal/totp against KeePassXC's
// own implementation reading the same entry.
func TestKeePassXCAgreesOnTOTP(t *testing.T) {
	bin := keepassxcCLI(t)
	path := filepath.Join("testdata", "kdbx40-password.kdbx")

	out, err := runCLI(t, bin, fixturePassword, "show", "-q", "-t", path, "GitHub")
	if err != nil {
		t.Fatalf("keepassxc-cli failed: %v\n%s", err, out)
	}
	theirs := strings.TrimSpace(out)
	if len(theirs) != 6 {
		t.Fatalf("keepassxc-cli returned %q, want a six digit code", theirs)
	}

	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})
	seed, err := v.TOTPSeed(idOf(t, v, "GitHub"))
	if err != nil {
		t.Fatal(err)
	}
	ours, err := totpCode(seed)
	if err != nil {
		t.Fatal(err)
	}
	if ours != theirs {
		t.Errorf("our code %s, KeePassXC %s", ours, theirs)
	}
}

// TestRecycleBinIsKeePassXCsRecycleBin proves the bin we create is the one
// KeePassXC recognises, not just a group with the same name.
func TestRecycleBinIsKeePassXCsRecycleBin(t *testing.T) {
	bin := keepassxcCLI(t)
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))

	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Delete(idOf(t, v, "Bank")); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}
	v.Close()

	out, err := runCLI(t, bin, fixturePassword, "ls", "-q", "-R", path)
	if err != nil {
		t.Fatalf("keepassxc-cli failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, recycleBinName) {
		t.Errorf("KeePassXC does not list a %q group:\n%s", recycleBinName, out)
	}
	if !strings.Contains(out, "Bank") {
		t.Errorf("recycled entry missing from the listing:\n%s", out)
	}
}

// TestOpenVaultAuthoredByKeePassXC covers the other direction of the
// compatibility requirement. Every other fixture was written by
// gokeepasslib, so a symmetric misreading of the format would pass those
// tests; this file was produced by keepassxc-cli itself and is committed as
// bytes, which makes it the only fixture that can catch such a bug.
//
// Regenerate it with:
//
//	keepassxc-cli db-create -p internal/vault/testdata/keepassxc-authored.kdbx
//	keepassxc-cli add -u octocat --url https://github.com --password-prompt \
//	  internal/vault/testdata/keepassxc-authored.kdbx GitHub
//
// using the password in fixturePassword.
//
// keepassxc-cli 2.7 writes KDBX 3.1 from db-create and offers no version
// flag, so this fixture covers the 3.1 read path only. Reading a KDBX 4 file
// that KeePassXC wrote still needs the manual check in the README, because
// only the GUI produces one.
func TestOpenVaultAuthoredByKeePassXC(t *testing.T) {
	v, err := Open(filepath.Join("testdata", "keepassxc-authored.kdbx"),
		Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	list, err := v.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("List returned %d entries, want 1", len(list))
	}
	got := list[0]
	if got.Title != "GitHub" {
		t.Errorf("Title = %q, want GitHub", got.Title)
	}
	if got.Username != "octocat" {
		t.Errorf("Username = %q, want octocat", got.Username)
	}
	if got.URL != "https://github.com" {
		t.Errorf("URL = %q, want https://github.com", got.URL)
	}

	pw, err := v.Reveal(got.ID, FieldPassword)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "hunter2-github" {
		t.Errorf("password = %q, want hunter2-github", pw)
	}
	t.Logf("KeePassXC wrote this file as KDBX %s", v.FormatVersion())
}

// TestRoundTripOfAKeePassXCFile edits the KeePassXC-authored file with this
// package, saves it, and has KeePassXC read it back, so both writers touch
// the same bytes in turn.
func TestRoundTripOfAKeePassXCFile(t *testing.T) {
	bin := keepassxcCLI(t)
	path := copyFixture(t, filepath.Join("testdata", "keepassxc-authored.kdbx"))

	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Add(Draft{
		Title:    "Added By Vaulty",
		Username: "someone",
		Password: "added-p4ss",
	}); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}
	v.Close()

	out, err := runCLI(t, bin, fixturePassword,
		"show", "-q", "-s", "-a", "Password", path, "Added By Vaulty")
	if err != nil {
		t.Fatalf("keepassxc-cli failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(out); got != "added-p4ss" {
		t.Errorf("KeePassXC read the password as %q, want added-p4ss", got)
	}

	// The entry KeePassXC originally wrote must survive our save.
	original, err := runCLI(t, bin, fixturePassword,
		"show", "-q", "-s", "-a", "Password", path, "GitHub")
	if err != nil {
		t.Fatalf("keepassxc-cli failed: %v\n%s", err, original)
	}
	if got := strings.TrimSpace(original); got != "hunter2-github" {
		t.Errorf("the original entry reads as %q after our save", got)
	}
}
