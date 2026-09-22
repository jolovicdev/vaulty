package importer

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jolovicdev/vaulty/internal/vault"
)

// The fixtures in testdata are written by hand in each app's export format,
// with every kind of value the readers map. The archive formats are zipped
// here from those files, so the JSON inside them stays readable in review.

func TestProtonPassZip(t *testing.T) {
	path := zipFixture(t, "export.zip", map[string]string{
		"Proton Pass/data.json":           read(t, "proton.json"),
		"Proton Pass/files/share-a/1.png": "png",
	})
	res := mustRead(t, path)

	if res.Source != "Proton Pass" {
		t.Errorf("Source = %q", res.Source)
	}
	if len(res.Entries) != 5 {
		t.Fatalf("got %d entries, want 5 with the trashed one left out", len(res.Entries))
	}
	wantWarnings(t, res, "1 item in the trash was left out", "1 passkey was not imported", "1 attached file was not imported")

	gh := find(t, res, "GitHub")
	wantFolder(t, gh, "Work")
	wantDraft(t, gh.Draft, "octocat", "hunter2-proton", "https://github.com/login")
	wantSeed(t, gh.Draft, "otpauth://totp/GitHub:octocat?secret=JBSWY3DPEHPK3PXP&issuer=GitHub")
	wantField(t, gh.Draft, "KP2A_URL_1", "https://github.com/enterprise", false)
	// The item lists its URLs twice, in urls and in autofillUrls.
	if _, ok := gh.Draft.Custom["KP2A_URL_2"]; ok {
		t.Error("a URL listed twice was stored twice")
	}
	wantField(t, gh.Draft, "Email", "me@example.com", false)
	wantField(t, gh.Draft, "Recovery code", "rc-1111", true)
	wantField(t, gh.Draft, "Team", "platform", false)
	wantField(t, gh.Draft, "One-time password", "otpauth://totp/backup?secret=GEZDGNBVGY3TQOJQ", true)
	if gh.Draft.Notes != "work account" {
		t.Errorf("Notes = %q", gh.Draft.Notes)
	}

	// No username, so the email stands in; the bare seed is given a URI.
	bank := find(t, res, "Bank")
	wantDraft(t, bank.Draft, "saver@example.com", "correct-horse-proton", "https://bank.example")
	wantSeed(t, bank.Draft, "otpauth://totp/Bank?secret=JBSWY3DPEHPK3PXP")

	visa := find(t, res, "Visa")
	wantFolder(t, visa, "Personal")
	wantField(t, visa.Draft, "Number", "4242424242424242", true)
	wantField(t, visa.Draft, "Verification number", "123", true)
	wantField(t, visa.Draft, "Pin", "9876", true)
	wantField(t, visa.Draft, "Cardholder name", "Jo Doe", false)

	me := find(t, res, "Me")
	wantField(t, me.Draft, "Social security number", "078-05-1120", true)
	wantField(t, me.Draft, "Nickname", "JD", false)
	wantField(t, me.Draft, "Frequent flyer", "FF-555", true)

	wantDraft(t, find(t, res, "Shop alias").Draft, "shop.alias@passmail.net", "", "")
}

func TestProtonPassCSV(t *testing.T) {
	res := mustRead(t, filepath.Join("testdata", "proton.csv"))

	gh := find(t, res, "GitHub")
	wantFolder(t, gh, "Work")
	wantDraft(t, gh.Draft, "octocat", "hunter2-proton", "https://github.com/login")
	wantField(t, gh.Draft, "KP2A_URL_1", "https://github.com/enterprise", false)
	wantField(t, gh.Draft, "Email", "me@example.com", false)
	wantSeed(t, gh.Draft, "otpauth://totp/GitHub:octocat?secret=JBSWY3DPEHPK3PXP&issuer=GitHub")

	// The card's fields arrive as JSON in the note column.
	visa := find(t, res, "Visa")
	wantField(t, visa.Draft, "Number", "4242424242424242", true)
	wantField(t, visa.Draft, "Pin", "9876", true)
	if visa.Draft.Notes != "daily card" {
		t.Errorf("Notes = %q, want the note from inside the JSON", visa.Draft.Notes)
	}
}

func TestEncryptedExportsAreRefused(t *testing.T) {
	dir := t.TempDir()
	protonJSON := filepath.Join(dir, "proton.json")
	write(t, protonJSON, `{"version": "1.21.2", "encrypted": true, "vaults": {}}`)
	proton := zipFixture(t, "proton.zip", map[string]string{"Proton Pass/data.pgp": "-----BEGIN PGP MESSAGE-----"})

	for _, path := range []string{protonJSON, proton} {
		if _, err := Read(path); !errors.Is(err, ErrEncrypted) {
			t.Errorf("%s: err = %v, want ErrEncrypted", filepath.Base(path), err)
		}
	}
}

func TestOtherFilesAreRefused(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "other.csv")
	write(t, csvPath, "a,b\n1,2\n")
	jsonPath := filepath.Join(dir, "other.json")
	write(t, jsonPath, `{"hello": "world"}`)
	zipPath := zipFixture(t, "other.zip", map[string]string{"readme.txt": "hi"})

	for _, path := range []string{csvPath, jsonPath, zipPath} {
		if _, err := Read(path); !errors.Is(err, ErrUnknownFormat) {
			t.Errorf("%s: err = %v, want ErrUnknownFormat", filepath.Base(path), err)
		}
	}
}

// TestFieldNamesNeverShadowStandardFields matters because applyDraft skips
// a custom field named like a standard one, which would lose its value.
func TestFieldNamesNeverShadowStandardFields(t *testing.T) {
	e := newEntry(nil, "t")
	e.field("Password", "a", true)
	e.field("Password", "b", false)
	e.field("", "c", false)
	e.field("Empty", "  ", false)

	want := map[string]string{"Password 2": "a", "Password 3": "b", "Field": "c"}
	if !reflect.DeepEqual(e.d.Custom, want) {
		t.Errorf("Custom = %v, want %v", e.d.Custom, want)
	}
	if !reflect.DeepEqual(e.d.ProtectedCustom, []string{"Password 2"}) {
		t.Errorf("ProtectedCustom = %v", e.d.ProtectedCustom)
	}
}

func mustRead(t *testing.T, path string) Result {
	t.Helper()
	res, err := Read(path)
	if err != nil {
		t.Fatalf("Read %s: %v", filepath.Base(path), err)
	}
	return res
}

func read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func zipFixture(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, content := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func find(t *testing.T, res Result, title string) vault.Imported {
	t.Helper()
	for _, e := range res.Entries {
		if e.Draft.Title == title {
			return e
		}
	}
	t.Fatalf("no entry titled %q", title)
	return vault.Imported{}
}

func wantFolder(t *testing.T, e vault.Imported, folder ...string) {
	t.Helper()
	if strings.Join(e.Folder, "/") != strings.Join(folder, "/") {
		t.Errorf("%s: Folder = %q, want %q", e.Draft.Title, e.Folder, folder)
	}
}

func wantDraft(t *testing.T, d vault.Draft, username, password, url string) {
	t.Helper()
	if d.Username != username || d.Password != password || d.URL != url {
		t.Errorf("%s: username, password, URL = %q, %q, %q; want %q, %q, %q",
			d.Title, d.Username, d.Password, d.URL, username, password, url)
	}
}

func wantSeed(t *testing.T, d vault.Draft, seed string) {
	t.Helper()
	if d.TOTPSeed != seed {
		t.Errorf("%s: TOTPSeed = %q, want %q", d.Title, d.TOTPSeed, seed)
	}
}

func wantField(t *testing.T, d vault.Draft, key, value string, protected bool) {
	t.Helper()
	got, ok := d.Custom[key]
	if !ok {
		t.Errorf("%s: no field %q in %v", d.Title, key, d.Custom)
		return
	}
	if got != value {
		t.Errorf("%s: field %q = %q, want %q", d.Title, key, got, value)
	}
	isProtected := false
	for _, k := range d.ProtectedCustom {
		isProtected = isProtected || k == key
	}
	if isProtected != protected {
		t.Errorf("%s: field %q protected = %v, want %v", d.Title, key, isProtected, protected)
	}
}

func wantWarnings(t *testing.T, res Result, warnings ...string) {
	t.Helper()
	if !reflect.DeepEqual(res.Warnings, warnings) {
		t.Errorf("Warnings = %q, want %q", res.Warnings, warnings)
	}
}
