package vault

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAddUpdateDelete(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")

	id, err := v.Add(Draft{
		Title:    "Lifecycle",
		Username: "user",
		Password: "first-password",
		Tags:     []string{"temp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Dirty() {
		t.Error("Dirty = false after Add")
	}

	if err := v.Update(id, Draft{Title: "Lifecycle", Username: "user2", Password: "second-password"}); err != nil {
		t.Fatal(err)
	}
	got, err := v.Reveal(id, FieldPassword)
	if err != nil {
		t.Fatal(err)
	}
	if got != "second-password" {
		t.Errorf("password after Update = %q", got)
	}

	d, err := v.Detail(id)
	if err != nil {
		t.Fatal(err)
	}
	if d.HistoryCount != 1 {
		t.Errorf("HistoryCount = %d, want 1", d.HistoryCount)
	}
	if d.Username != "user2" {
		t.Errorf("Username = %q, want user2", d.Username)
	}

	// The first delete recycles, so the entry still exists.
	if err := v.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Detail(id); err != nil {
		t.Fatalf("entry gone after the first Delete: %v", err)
	}
	d, err = v.Detail(id)
	if err != nil {
		t.Fatal(err)
	}
	if !d.InRecycleBin {
		t.Error("InRecycleBin = false after Delete")
	}

	// The second delete is permanent.
	if err := v.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Detail(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("Detail after the second Delete = %v, want ErrNotFound", err)
	}
	if len(v.db.Content.Root.DeletedObjects) == 0 {
		t.Error("no DeletedObject recorded for a permanent delete")
	}
}

func TestDeletedEntryLeavesTheDefaultList(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	id := idOf(t, v, "Bank")

	if err := v.Delete(id); err != nil {
		t.Fatal(err)
	}

	plain, err := v.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range plain {
		if m.ID == id {
			t.Error("recycled entry still appears in the default list")
		}
	}

	withBin, err := v.List(ListOptions{IncludeRecycleBin: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range withBin {
		if m.ID == id {
			found = true
			if !m.InRecycleBin {
				t.Error("InRecycleBin = false for a recycled entry")
			}
		}
	}
	if !found {
		t.Error("recycled entry missing even with IncludeRecycleBin")
	}
}

func TestRecycleBinMatchesKeePassXCLayout(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	if err := v.Delete(idOf(t, v, "Bank")); err != nil {
		t.Fatal(err)
	}

	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	var bin *GroupNode
	for i := range groups[0].Children {
		if groups[0].Children[i].IsRecycle {
			bin = &groups[0].Children[i]
		}
	}
	if bin == nil {
		t.Fatal("no group flagged as the recycle bin")
	}
	if bin.Name != recycleBinName {
		t.Errorf("bin name = %q, want %q", bin.Name, recycleBinName)
	}
	if v.db.Content.Meta.RecycleBinUUID.IsZero() {
		t.Error("RecycleBinUUID not recorded in the metadata")
	}
	if v.db.Content.Meta.RecycleBinChanged == nil {
		t.Error("RecycleBinChanged not stamped")
	}
}

func TestRestoreFromRecycleBin(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	id := idOf(t, v, "Bank")
	if err := v.Delete(id); err != nil {
		t.Fatal(err)
	}

	if err := v.Restore(id, ""); err != nil {
		t.Fatal(err)
	}
	d, err := v.Detail(id)
	if err != nil {
		t.Fatal(err)
	}
	if d.InRecycleBin {
		t.Error("InRecycleBin = true after Restore")
	}
	// The password must survive the trip to the bin and back.
	got, err := v.Reveal(id, FieldPassword)
	if err != nil {
		t.Fatal(err)
	}
	if got != "correct-horse-bank" {
		t.Errorf("password after Restore = %q", got)
	}
}

func TestEmptyRecycleBin(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	if err := v.Delete(idOf(t, v, "Bank")); err != nil {
		t.Fatal(err)
	}
	if err := v.EmptyRecycleBin(); err != nil {
		t.Fatal(err)
	}

	all, err := v.List(ListOptions{IncludeRecycleBin: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.InRecycleBin {
			t.Errorf("entry %q still in the bin", m.Title)
		}
	}
	if len(v.db.Content.Root.DeletedObjects) == 0 {
		t.Error("EmptyRecycleBin recorded no tombstones")
	}
}

func TestMoveBetweenGroups(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	work := groups[0].Children[0].ID
	id := idOf(t, v, "Bank")

	if err := v.Move(id, work); err != nil {
		t.Fatal(err)
	}
	d, err := v.Detail(id)
	if err != nil {
		t.Fatal(err)
	}
	if d.GroupID != work {
		t.Errorf("GroupID = %q, want %q", d.GroupID, work)
	}
	if d.GroupPath != "vaulty fixture/Work" {
		t.Errorf("GroupPath = %q", d.GroupPath)
	}

	if err := v.Move(id, "no-such-group"); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("Move to a missing group = %v, want ErrGroupNotFound", err)
	}
}

func TestAddGroup(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}

	id, err := v.AddGroup(groups[0].ID, "Servers")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("AddGroup returned an empty id")
	}

	after, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range after[0].Children {
		if c.ID == id && c.Name == "Servers" {
			found = true
		}
	}
	if !found {
		t.Error("new group not in the tree")
	}
}

func TestHistoryAndRestoreVersion(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	id := idOf(t, v, "GitHub")

	if err := v.Update(id, Draft{Title: "GitHub", Username: "v2", Password: "pass-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := v.Update(id, Draft{Title: "GitHub", Username: "v3", Password: "pass-v3"}); err != nil {
		t.Fatal(err)
	}

	versions, err := v.History(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("History = %d versions, want 2", len(versions))
	}
	if versions[0].Username != "octocat" {
		t.Errorf("oldest version username = %q, want octocat", versions[0].Username)
	}
	if versions[1].Username != "v2" {
		t.Errorf("second version username = %q, want v2", versions[1].Username)
	}

	// A history listing must not carry secrets either.
	pw, err := v.RevealHistory(id, 0, FieldPassword)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "hunter2-github" {
		t.Errorf("RevealHistory = %q, want the original password", pw)
	}

	if err := v.RestoreVersion(id, 0); err != nil {
		t.Fatal(err)
	}
	got, err := v.Reveal(id, FieldPassword)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2-github" {
		t.Errorf("password after RestoreVersion = %q", got)
	}

	// Restoring is itself recorded, so the user can undo the undo.
	after, err := v.History(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 3 {
		t.Errorf("History after restore = %d, want 3", len(after))
	}
}

func TestHistorySurvivesSave(t *testing.T) {
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))
	creds := Credentials{Password: fixturePassword}

	v, err := Open(path, creds)
	if err != nil {
		t.Fatal(err)
	}
	id := idOf(t, v, "GitHub")
	if err := v.Update(id, Draft{Title: "GitHub", Username: "newer", Password: "newer-pass"}); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}
	v.Close()

	again, err := Open(path, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()

	versions, err := again.History(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("History after reopen = %d, want 1", len(versions))
	}
	pw, err := again.RevealHistory(id, 0, FieldPassword)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "hunter2-github" {
		t.Errorf("historic password after reopen = %q", pw)
	}
}

func openWritable(t *testing.T, name string) *Vault {
	t.Helper()
	path := copyFixture(t, filepath.Join("testdata", name))
	v, err := Open(path, Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatalf("Open %s: %v", name, err)
	}
	t.Cleanup(v.Close)
	return v
}

// TestUpdateMovesEntryWhenTheGroupChanges covers the editor's group picker:
// a draft that names a different group has to relocate the entry, not just
// rewrite its fields.
func TestUpdateMovesEntryWhenTheGroupChanges(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	root, work := groups[0].ID, groups[0].Children[0].ID
	id := idOf(t, v, "Bank")

	cases := []struct {
		name      string
		groupID   string
		wantGroup string
		wantPath  string
	}{
		{"into a subgroup", work, work, "vaulty fixture/Work"},
		{"back to the root", root, root, "vaulty fixture"},
		{"unchanged when the draft repeats the group", root, root, "vaulty fixture"},
		{"unchanged when the draft names no group", "", root, "vaulty fixture"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := v.Update(id, Draft{
				Title:    "Bank",
				Password: "still-the-bank",
				GroupID:  c.groupID,
			}); err != nil {
				t.Fatal(err)
			}
			d, err := v.Detail(id)
			if err != nil {
				t.Fatal(err)
			}
			if d.GroupID != c.wantGroup {
				t.Errorf("GroupID = %q, want %q", d.GroupID, c.wantGroup)
			}
			if d.GroupPath != c.wantPath {
				t.Errorf("GroupPath = %q, want %q", d.GroupPath, c.wantPath)
			}
			// The move must not cost the entry its fields.
			pw, err := v.Reveal(id, FieldPassword)
			if err != nil {
				t.Fatal(err)
			}
			if pw != "still-the-bank" {
				t.Errorf("password = %q after the update", pw)
			}
		})
	}
}

// TestUpdateKeepsTheTOTPSeedTheDraftLeavesEmpty covers the editor's one-time
// code field, which never loads the stored seed and sends an empty one to
// mean unchanged.
func TestUpdateKeepsTheTOTPSeedTheDraftLeavesEmpty(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	id := idOf(t, v, "GitHub")
	before, err := v.TOTPSeed(id)
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Update(id, Draft{Title: "GitHub renamed", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	after, err := v.TOTPSeed(id)
	if err != nil {
		t.Fatalf("TOTPSeed after an edit that left it empty: %v", err)
	}
	if after != before {
		t.Errorf("seed = %q, want %q", after, before)
	}

	// A draft that does carry a seed replaces the stored one.
	const replacement = "otpauth://totp/GitHub:octocat?secret=GEZDGNBVGY3TQOJQ"
	if err := v.Update(id, Draft{Title: "GitHub renamed", Password: "p", TOTPSeed: replacement}); err != nil {
		t.Fatal(err)
	}
	if got, err := v.TOTPSeed(id); err != nil || got != replacement {
		t.Errorf("seed = %q, %v, want the replacement", got, err)
	}
}

// TestUpdateRemovesTheOneTimeCodeWhenAsked covers the editor's Remove toggle.
// An empty seed means unchanged, so taking the code off an entry needs its
// own flag, and the flag has to clear both shapes a seed is stored in.
func TestUpdateRemovesTheOneTimeCodeWhenAsked(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")

	// The editor sends the older seed and settings pair back as the custom
	// fields they are.
	pair := map[string]string{"TOTP Seed": "JBSWY3DPEHPK3PXP", "TOTP Settings": "30;6", "Account ID": "42"}
	legacy, err := v.Add(Draft{Title: "Legacy", Custom: pair})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		id    string
		draft Draft
	}{
		{"an otpauth URI in the otp field", idOf(t, v, "GitHub"), Draft{Title: "GitHub", RemoveTOTP: true}},
		{"the older seed and settings pair", legacy, Draft{Title: "Legacy", Custom: pair, RemoveTOTP: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := v.TOTPSeed(c.id); err != nil {
				t.Fatalf("no seed before the edit: %v", err)
			}
			if err := v.Update(c.id, c.draft); err != nil {
				t.Fatal(err)
			}
			if _, err := v.TOTPSeed(c.id); !errors.Is(err, ErrNotFound) {
				t.Errorf("TOTPSeed after the removal = %v, want ErrNotFound", err)
			}
			d, err := v.Detail(c.id)
			if err != nil {
				t.Fatal(err)
			}
			if d.HasTOTP {
				t.Error("HasTOTP = true after the removal")
			}
		})
	}

	// Only the seed goes. The custom field beside it stays.
	if got, err := v.Reveal(legacy, "Account ID"); err != nil || got != "42" {
		t.Errorf("Account ID = %q, %v, want it untouched", got, err)
	}
}
