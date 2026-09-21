package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tobischo/gokeepasslib/v3"
)

func TestOpenFixtures(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			v, err := Open(f.file, f.creds)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer v.Close()

			if got := v.FormatVersion(); got != f.version {
				t.Errorf("FormatVersion = %q, want %q", got, f.version)
			}
			list, err := v.List(ListOptions{})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(list) != 3 {
				t.Fatalf("List returned %d entries, want 3", len(list))
			}
		})
	}
}

func TestOpenWrongCredentials(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		creds Credentials
	}{
		{"wrong password", "kdbx40-password.kdbx", Credentials{Password: "nope"}},
		{"missing key file", "kdbx40-password-keyfile.kdbx", Credentials{Password: fixturePassword}},
		{"no credentials at all", "kdbx40-password.kdbx", Credentials{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := Open(filepath.Join("testdata", c.file), c.creds)
			if err == nil {
				v.Close()
				t.Fatal("Open succeeded, want an error")
			}
			if strings.Contains(err.Error(), fixturePassword) {
				t.Errorf("error text leaks the password: %v", err)
			}
		})
	}
}

// TestListCarriesNoSecrets checks that the entry list stays free of secrets:
// nothing in a list payload may equal a stored password, note or protected
// custom field.
func TestListCarriesNoSecrets(t *testing.T) {
	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})

	list, err := v.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	secrets := []string{"hunter2-github", "correct-horse-bank", "jira-pass-01",
		"notes for GitHub", "tok-GitHub", "JBSWY3DPEHPK3PXP"}
	for _, m := range list {
		fields := []string{m.ID, m.Title, m.Username, m.URL, m.GroupID, m.GroupPath}
		fields = append(fields, m.Tags...)
		for _, f := range fields {
			for _, s := range secrets {
				if f == s || (s != "" && strings.Contains(f, s)) {
					t.Errorf("list field %q contains secret %q", f, s)
				}
			}
		}
	}
}

func TestDetailHidesProtectedFields(t *testing.T) {
	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})
	id := idOf(t, v, "GitHub")

	d, err := v.Detail(id)
	if err != nil {
		t.Fatal(err)
	}
	if !d.PasswordSet {
		t.Error("PasswordSet = false, want true")
	}
	if !d.HasTOTP {
		t.Error("HasTOTP = false, want true")
	}

	byKey := map[string]CustomField{}
	for _, f := range d.Custom {
		byKey[f.Key] = f
	}
	if got, ok := byKey["API Token"]; !ok {
		t.Fatal("custom field API Token missing")
	} else {
		if !got.Protected {
			t.Error("API Token Protected = false, want true")
		}
		if got.Value != "" {
			t.Errorf("API Token Value = %q, want empty for a protected field", got.Value)
		}
	}
	if got := byKey["Account ID"]; got.Value != "acct-GitHub" {
		t.Errorf("Account ID Value = %q, want acct-GitHub", got.Value)
	}
}

func TestReveal(t *testing.T) {
	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})
	id := idOf(t, v, "GitHub")

	cases := []struct {
		field string
		want  string
		err   error
	}{
		{FieldPassword, "hunter2-github", nil},
		{FieldNotes, "notes for GitHub", nil},
		{"API Token", "tok-GitHub", nil},
		{"Nope", "", ErrNotFound},
		{"", "", ErrReadOnlyField},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			got, err := v.Reveal(id, c.field)
			if !errors.Is(err, c.err) {
				t.Fatalf("err = %v, want %v", err, c.err)
			}
			if got != c.want {
				t.Errorf("Reveal = %q, want %q", got, c.want)
			}
		})
	}
}

// TestRoundTrip is the M1 acceptance check: open, modify, save, reopen, and
// compare every field that was written.
func TestRoundTrip(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			path := copyFixture(t, f.file)
			creds := f.creds
			if creds.KeyFile != "" {
				creds.KeyFile = keyFileCopy(t)
			}

			v, err := Open(path, creds)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			draft := Draft{
				Title:           "Round Trip",
				Username:        "rt-user",
				Password:        "rt-password-éü中",
				URL:             "https://round.trip/path?q=1",
				Notes:           "line one\nline two",
				Tags:            []string{"alpha", "beta"},
				TOTPSeed:        "otpauth://totp/RT?secret=JBSWY3DPEHPK3PXP",
				Custom:          map[string]string{"Secret Key": "s3cr3t", "Plain": "visible"},
				ProtectedCustom: []string{"Secret Key"},
			}
			id, err := v.Add(draft)
			if err != nil {
				t.Fatalf("Add: %v", err)
			}
			if err := v.Save(); err != nil {
				t.Fatalf("Save: %v", err)
			}
			v.Close()

			again, err := Open(path, creds)
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			defer again.Close()

			if got := again.FormatVersion(); got != f.version {
				t.Errorf("format changed across save: %q, want %q", got, f.version)
			}

			d, err := again.Detail(id)
			if err != nil {
				t.Fatalf("Detail after reopen: %v", err)
			}
			if d.Title != draft.Title {
				t.Errorf("Title = %q, want %q", d.Title, draft.Title)
			}
			if d.Username != draft.Username {
				t.Errorf("Username = %q, want %q", d.Username, draft.Username)
			}
			if d.URL != draft.URL {
				t.Errorf("URL = %q, want %q", d.URL, draft.URL)
			}
			sort.Strings(d.Tags)
			if strings.Join(d.Tags, ",") != "alpha,beta" {
				t.Errorf("Tags = %v, want [alpha beta]", d.Tags)
			}
			if !d.HasTOTP {
				t.Error("HasTOTP = false after round trip")
			}

			for field, want := range map[string]string{
				FieldPassword: draft.Password,
				FieldNotes:    draft.Notes,
				"Secret Key":  "s3cr3t",
				"Plain":       "visible",
			} {
				got, err := again.Reveal(id, field)
				if err != nil {
					t.Fatalf("Reveal %s: %v", field, err)
				}
				if got != want {
					t.Errorf("Reveal %s = %q, want %q", field, got, want)
				}
			}

			// A protected field must still be protected after the trip, or
			// KeePassXC would show the value in plain text.
			for _, c := range d.Custom {
				if c.Key == "Secret Key" && !c.Protected {
					t.Error("Secret Key lost its protected flag")
				}
				if c.Key == "Plain" && c.Protected {
					t.Error("Plain gained a protected flag")
				}
			}
		})
	}
}

func TestSearch(t *testing.T) {
	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"exact title", "GitHub", []string{"GitHub"}},
		{"title prefix", "git", []string{"GitHub"}},
		{"case insensitive", "BANK", []string{"Bank"}},
		{"by username", "octocat", []string{"GitHub"}},
		{"by url host", "jira.example", []string{"Jira"}},
		{"by tag", "finance", []string{"Bank"}},
		{"subsequence", "jra", []string{"Jira"}},
		{"no match", "zzzzz", nil},
		{"empty lists all", "", []string{"Bank", "GitHub", "Jira"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := v.Search(c.query, ListOptions{})
			if err != nil {
				t.Fatal(err)
			}
			titles := make([]string, len(got))
			for i, m := range got {
				titles[i] = m.Title
			}
			if len(c.want) == 0 {
				if len(titles) != 0 {
					t.Fatalf("got %v, want no results", titles)
				}
				return
			}
			if len(titles) < len(c.want) {
				t.Fatalf("got %v, want at least %v", titles, c.want)
			}
			// The first len(want) results must be exactly want, in order.
			for i, w := range c.want {
				if titles[i] != w {
					t.Errorf("result %d = %q, want %q (full: %v)", i, titles[i], w, titles)
				}
			}
		})
	}
}

// TestSearchRanksTitleAboveTag pins the weighting: "code" is a tag on both
// GitHub and Jira, but a title match must outrank a tag match.
func TestSearchRanksTitleAboveTag(t *testing.T) {
	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})
	got, err := v.Search("jira", ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Title != "Jira" {
		t.Fatalf("first result = %+v, want Jira", got)
	}
	if got[0].Score <= 0 {
		t.Errorf("Score = %d, want positive", got[0].Score)
	}
}

// TestMatchScoreOrdersTheKindsOfMatch pins the ladder matchScore documents.
// The start of a word has to beat a hit in the middle of one, and every word
// start is also a substring, so the word test has to be tried first.
func TestMatchScoreOrdersTheKindsOfMatch(t *testing.T) {
	cases := []struct {
		needle   string
		haystack string
		want     int
	}{
		{"github", "github", 10},
		{"git", "github", 8},
		{"hub", "git-hub", 6},
		{"hub", "github", 5},
		{"gtb", "github", 2},
		{"zzz", "github", 0},
	}
	for _, c := range cases {
		if got := matchScore(c.needle, c.haystack); got != c.want {
			t.Errorf("matchScore(%q, %q) = %d, want %d", c.needle, c.haystack, got, c.want)
		}
	}
}

func TestGroupsAndTags(t *testing.T) {
	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})

	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("root groups = %d, want 1", len(groups))
	}
	if groups[0].Count != 2 {
		t.Errorf("root entry count = %d, want 2", groups[0].Count)
	}
	if len(groups[0].Children) != 1 || groups[0].Children[0].Name != "Work" {
		t.Fatalf("children = %+v, want one group named Work", groups[0].Children)
	}
	if got := groups[0].Children[0].Path; got != "vaulty fixture/Work" {
		t.Errorf("Work path = %q", got)
	}

	tags, err := v.Tags()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tags, ",") != "code,finance,ops,work" {
		t.Errorf("Tags = %v", tags)
	}
}

func TestListFilters(t *testing.T) {
	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})
	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	work := groups[0].Children[0].ID

	cases := []struct {
		name string
		opts ListOptions
		want int
	}{
		{"all", ListOptions{}, 3},
		{"by group", ListOptions{GroupID: work}, 1},
		{"by tag", ListOptions{Tag: "code"}, 2},
		{"group and tag", ListOptions{GroupID: work, Tag: "finance"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := v.List(c.opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != c.want {
				t.Errorf("List = %d entries, want %d", len(got), c.want)
			}
		})
	}
}

func TestCreateNewVaultIsKdbx4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.kdbx")
	v, err := Create(path, Credentials{Password: "a new master password"})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.FormatVersion(); got != "4.0" {
		t.Errorf("FormatVersion = %q, want 4.0", got)
	}
	v.Close()

	// Creating over an existing file must fail rather than destroy it.
	if _, err := Create(path, Credentials{Password: "x"}); err == nil {
		t.Error("Create over an existing file succeeded, want an error")
	}

	again, err := Open(path, Credentials{Password: "a new master password"})
	if err != nil {
		t.Fatalf("reopen created vault: %v", err)
	}
	defer again.Close()
	if again.DatabaseName() != "new" {
		t.Errorf("DatabaseName = %q, want new", again.DatabaseName())
	}
}

// TestCreateSetsTheKDFCost reads the parameters back from the file. The
// header gokeepasslib gives a new database costs 1 MiB and two passes, which
// lets whoever holds the file try a password every few milliseconds.
func TestCreateSetsTheKDFCost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kdf.kdbx")
	v, err := Create(path, Credentials{Password: "master"})
	if err != nil {
		t.Fatal(err)
	}
	v.Close()

	again, err := Open(path, Credentials{Password: "master"})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()

	kdf := again.db.Header.FileHeaders.KdfParameters
	if !bytes.Equal(kdf.UUID, gokeepasslib.KdfArgon2) {
		t.Errorf("KDF UUID = %x, want Argon2d %x", kdf.UUID, gokeepasslib.KdfArgon2)
	}
	if kdf.Memory != 64<<20 {
		t.Errorf("Memory = %d bytes, want 64 MiB", kdf.Memory)
	}
	if kdf.Iterations != 10 {
		t.Errorf("Iterations = %d, want 10", kdf.Iterations)
	}
}

func TestCloseDropsState(t *testing.T) {
	v := openFixture(t, "kdbx40-password.kdbx", Credentials{Password: fixturePassword})
	id := idOf(t, v, "GitHub")
	v.Close()

	if _, err := v.List(ListOptions{}); !errors.Is(err, ErrLocked) {
		t.Errorf("List after Close: %v, want ErrLocked", err)
	}
	if _, err := v.Reveal(id, FieldPassword); !errors.Is(err, ErrLocked) {
		t.Errorf("Reveal after Close: %v, want ErrLocked", err)
	}
	if _, err := v.Detail(id); !errors.Is(err, ErrLocked) {
		t.Errorf("Detail after Close: %v, want ErrLocked", err)
	}
}

// helpers

func openFixture(t *testing.T, name string, creds Credentials) *Vault {
	t.Helper()
	v, err := Open(filepath.Join("testdata", name), creds)
	if err != nil {
		t.Fatalf("Open %s: %v", name, err)
	}
	t.Cleanup(v.Close)
	return v
}

func idOf(t *testing.T, v *Vault, title string) string {
	t.Helper()
	list, err := v.List(ListOptions{IncludeRecycleBin: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		if m.Title == title {
			return m.ID
		}
	}
	t.Fatalf("no entry titled %q", title)
	return ""
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}

func advanceModTime(t *testing.T, path string) {
	t.Helper()
	fi := mustStat(t, path)
	future := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
}
