package vault

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/tobischo/gokeepasslib/v3"
	w "github.com/tobischo/gokeepasslib/v3/wrappers"
)

var regen = flag.Bool("regen", false, "rewrite the files in testdata")

// TestGenerateFixtures writes the testdata files. It is skipped unless
// -regen is given, so an ordinary test run reads the checked-in files rather
// than ones it just produced.
//
//	go test ./internal/vault -run TestGenerateFixtures -regen
func TestGenerateFixtures(t *testing.T) {
	if !*regen {
		t.Skip("run with -regen to rewrite testdata")
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join("testdata", "key.keyx")
	if err := os.WriteFile(keyPath, []byte(fixtureKeyData), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		file    string
		version gokeepasslib.DatabaseOption
		withKey bool
	}{
		{"kdbx31-password.kdbx", gokeepasslib.WithDatabaseKDBXVersion3(), false},
		{"kdbx31-password-keyfile.kdbx", gokeepasslib.WithDatabaseKDBXVersion3(), true},
		{"kdbx40-password.kdbx", gokeepasslib.WithDatabaseKDBXVersion40(), false},
		{"kdbx40-password-keyfile.kdbx", gokeepasslib.WithDatabaseKDBXVersion40(), true},
	}

	for _, c := range cases {
		path := filepath.Join("testdata", c.file)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}

		creds := Credentials{Password: fixturePassword}
		if c.withKey {
			creds.KeyFile = keyPath
		}
		dbc, err := creds.build()
		if err != nil {
			t.Fatal(err)
		}

		db := gokeepasslib.NewDatabase(c.version)
		db.Credentials = dbc
		db.Content.Meta.DatabaseName = "vaulty fixture"
		db.Content.Meta.RecycleBinEnabled = w.NewBoolWrapper(true)
		db.Content.Root.Groups = []gokeepasslib.Group{fixtureTree()}

		v := &Vault{path: path, db: db, dirty: true}
		if err := v.Save(); err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		t.Logf("wrote %s", path)
	}
}

// fixtureTree is the group and entry layout every fixture contains.
func fixtureTree() gokeepasslib.Group {
	root := gokeepasslib.NewGroup()
	root.Name = "vaulty fixture"

	work := gokeepasslib.NewGroup()
	work.Name = "Work"

	root.Entries = []gokeepasslib.Entry{
		fixtureEntry("GitHub", "octocat", "hunter2-github", "https://github.com", "ops;code",
			"otpauth://totp/GitHub:octocat?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"),
		fixtureEntry("Bank", "dusan", "correct-horse-bank", "https://bank.example", "finance", ""),
	}
	work.Entries = []gokeepasslib.Entry{
		fixtureEntry("Jira", "d.jolovic", "jira-pass-01", "https://jira.example/login", "work;code", ""),
	}
	root.Groups = []gokeepasslib.Group{work}
	return root
}

func fixtureEntry(title, user, pass, url, tags, otp string) gokeepasslib.Entry {
	e := gokeepasslib.NewEntry()
	e.Values = []gokeepasslib.ValueData{
		protectedValue(FieldTitle, title, false),
		protectedValue(FieldUserName, user, false),
		protectedValue(FieldPassword, pass, true),
		protectedValue(FieldURL, url, false),
		protectedValue(FieldNotes, "notes for "+title, false),
	}
	if otp != "" {
		e.Values = append(e.Values, protectedValue(FieldTOTPSeed, otp, true))
	}
	e.Values = append(e.Values,
		protectedValue("API Token", "tok-"+title, true),
		protectedValue("Account ID", "acct-"+title, false),
	)
	e.Tags = tags
	return e
}
