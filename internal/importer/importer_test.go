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
		"Proton Pass/data.json":           "\ufeff" + read(t, "proton.json"),
		"Proton Pass/files/share-a/1.png": "png",
	})
	res := mustRead(t, path)

	if res.Source != "Proton Pass" {
		t.Errorf("Source = %q", res.Source)
	}
	if len(res.Entries) != 6 {
		t.Fatalf("got %d entries, want 6 with the trashed one left out", len(res.Entries))
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

	wifi := find(t, res, "Home wifi")
	wantDraft(t, wifi.Draft, "", "wpa-secret", "")
	wantField(t, wifi.Draft, "Security", "1", false)

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

func TestBitwardenJSON(t *testing.T) {
	res := mustRead(t, filepath.Join("testdata", "bitwarden.json"))

	if res.Source != "Bitwarden" || len(res.Entries) != 3 {
		t.Fatalf("got %s with %d entries, want Bitwarden with 3", res.Source, len(res.Entries))
	}
	wantWarnings(t, res, "1 item in the trash was left out", "1 passkey was not imported")

	router := find(t, res, "Router")
	wantFolder(t, router, "Work", "Servers")
	wantDraft(t, router.Draft, "admin", "hunter2-bitwarden", "https://router.lan")
	wantSeed(t, router.Draft, "otpauth://totp/Router?secret=JBSWY3DPEHPK3PXP")
	wantField(t, router.Draft, "KP2A_URL_1", "https://router.backup.lan", false)
	wantField(t, router.Draft, "API key", "key-2222", true)
	wantField(t, router.Draft, "Region", "eu-west", false)
	wantField(t, router.Draft, "Enabled", "true", false)
	if _, ok := router.Draft.Custom["Linked"]; ok {
		t.Error("a linked field, which has no value, was imported")
	}

	amex := find(t, res, "Amex")
	wantFolder(t, amex)
	wantField(t, amex.Draft, "Number", "374242424242424", true)
	wantField(t, amex.Draft, "Code", "4321", true)
	wantField(t, amex.Draft, "Brand", "Amex", false)

	key := find(t, res, "Deploy key")
	wantField(t, key.Draft, "Private key", "-----BEGIN OPENSSH PRIVATE KEY-----\nfixture\n-----END OPENSSH PRIVATE KEY-----\n", true)
	wantField(t, key.Draft, "Public key", "ssh-ed25519 AAAAfixture", false)
}

func TestBitwardenOrganisationUsesCollections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "org.json")
	write(t, path, `{"encrypted": false,
		"collections": [{"id": "c1", "organizationId": "o1", "name": "Ops/Shared"}],
		"items": [{"type": 1, "name": "Shared login", "collectionIds": ["c1"],
			"login": {"username": "ops", "password": "p", "uris": []}}]}`)
	wantFolder(t, find(t, mustRead(t, path), "Shared login"), "Ops", "Shared")
}

func TestBitwardenCSV(t *testing.T) {
	res := mustRead(t, filepath.Join("testdata", "bitwarden.csv"))

	router := find(t, res, "Router")
	wantFolder(t, router, "Work", "Servers")
	wantDraft(t, router.Draft, "admin", "hunter2-bitwarden", "https://router.lan")
	wantField(t, router.Draft, "KP2A_URL_1", "https://router.backup.lan", false)
	wantField(t, router.Draft, "Region", "eu-west", false)
	wantField(t, router.Draft, "Rack", "2", false)
	wantField(t, router.Draft, "Hint", "ask: IT", false)
	wantSeed(t, router.Draft, "otpauth://totp/Router?secret=JBSWY3DPEHPK3PXP")

	wifi := find(t, res, "Wifi")
	wantFolder(t, wifi)
	if wifi.Draft.Notes != "the password is on the fridge" {
		t.Errorf("Notes = %q", wifi.Draft.Notes)
	}
}

func TestOnePassword1PUX(t *testing.T) {
	path := zipFixture(t, "export.1pux", map[string]string{
		"export.attributes": `{"version": 3, "description": "1Password Unencrypted Export"}`,
		"export.data":       read(t, "onepassword-export.data"),
		"files/doc.pdf":     "pdf",
	})
	res := mustRead(t, path)

	if res.Source != "1Password" || len(res.Entries) != 6 {
		t.Fatalf("got %s with %d entries, want 1Password with 6", res.Source, len(res.Entries))
	}
	wantWarnings(t, res, "1 attached file was not imported")

	ex := find(t, res, "Example")
	wantFolder(t, ex, "Private")
	wantDraft(t, ex.Draft, "jo@example.com", "hunter2-1password", "https://example.com")
	wantSeed(t, ex.Draft, "otpauth://totp/Example:jo?secret=JBSWY3DPEHPK3PXP&issuer=Example")
	wantField(t, ex.Draft, "KP2A_URL_1", "https://login.example.com", false)
	wantField(t, ex.Draft, "pin", "pin-4455", true)
	wantField(t, ex.Draft, "One-time password", "GEZDGNBVGY3TQOJQ", true)
	wantField(t, ex.Draft, "recovery key", "rk-7777", true)
	// An untitled field takes its section's title.
	wantField(t, ex.Draft, "Security", "ask IT", false)
	wantTags(t, ex.Draft, "work")
	if ex.Draft.Notes != "main account" {
		t.Errorf("Notes = %q", ex.Draft.Notes)
	}

	old := find(t, res, "Old wifi")
	wantDraft(t, old.Draft, "", "only-a-password", "")
	wantTags(t, old.Draft, "archived")

	// A server keeps its address and credentials in a section.
	db := find(t, res, "Database")
	wantFolder(t, db, "Infra")
	wantDraft(t, db.Draft, "postgres", "hunter2-server", "ssh://db.internal")

	visa := find(t, res, "Visa")
	wantField(t, visa.Draft, "number", "4242424242424242", true)
	wantField(t, visa.Draft, "expiry date", "04/2027", false)

	passport := find(t, res, "Passport")
	wantField(t, passport.Draft, "number", "X1234567", true)
	wantField(t, passport.Draft, "date of birth", "1990-01-01", false)

	bank := find(t, res, "Checking")
	wantField(t, bank.Draft, "account number", "12345678", true)
	wantField(t, bank.Draft, "telephone PIN", "2468", true)
	wantField(t, bank.Draft, "bank name", "First Bank", false)
}

func TestOnePasswordCSV(t *testing.T) {
	// The fixture starts with a byte order mark, as a Windows export does.
	res := mustRead(t, filepath.Join("testdata", "onepassword.csv"))

	ex := find(t, res, "Example")
	wantFolder(t, ex)
	wantDraft(t, ex.Draft, "jo@example.com", "hunter2-1password", "https://example.com")
	wantSeed(t, ex.Draft, "otpauth://totp/Example:jo?secret=JBSWY3DPEHPK3PXP&issuer=Example")
	wantTags(t, ex.Draft, "work", "finance")

	wantTags(t, find(t, res, "Old wifi").Draft, "archived")
}

func TestEncryptedExportsAreRefused(t *testing.T) {
	dir := t.TempDir()
	bitwarden := filepath.Join(dir, "bitwarden.json")
	write(t, bitwarden, `{"encrypted": true, "encKeyValidation_DO_NOT_EDIT": "2.x", "items": []}`)
	proton := zipFixture(t, "proton.zip", map[string]string{"Proton Pass/data.pgp": "-----BEGIN PGP MESSAGE-----"})

	for _, path := range []string{bitwarden, proton} {
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

func wantTags(t *testing.T, d vault.Draft, tags ...string) {
	t.Helper()
	if !reflect.DeepEqual(d.Tags, tags) {
		t.Errorf("%s: Tags = %q, want %q", d.Title, d.Tags, tags)
	}
}

func wantWarnings(t *testing.T, res Result, warnings ...string) {
	t.Helper()
	if !reflect.DeepEqual(res.Warnings, warnings) {
		t.Errorf("Warnings = %q, want %q", res.Warnings, warnings)
	}
}
