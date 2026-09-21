package vault

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	w "github.com/tobischo/gokeepasslib/v3/wrappers"
)

// twoCopies simulates the sync case: one vault opened twice from the same
// bytes, so both sides start with identical entry UUIDs.
func twoCopies(t *testing.T) (*Vault, *Vault) {
	t.Helper()
	creds := Credentials{Password: fixturePassword}
	src := filepath.Join("testdata", "kdbx40-password.kdbx")

	a, err := Open(copyFixture(t, src), creds)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	b, err := Open(copyFixture(t, src), creds)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return a, b
}

// setModTime forces an entry's modification time so the merge rule can be
// tested without sleeping.
func setModTime(t *testing.T, v *Vault, id string, at time.Time) {
	t.Helper()
	e, _, _, _, err := v.findEntry(id)
	if err != nil {
		t.Fatal(err)
	}
	tw := w.Now()
	tw.Time = at
	e.Times.LastModificationTime = &tw
}

func TestMergeNewerEntryWins(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		oursAt    time.Time
		theirsAt  time.Time
		wantPass  string
		wantCount MergeResult
	}{
		{
			name:      "theirs is newer",
			oursAt:    base,
			theirsAt:  base.Add(time.Hour),
			wantPass:  "theirs",
			wantCount: MergeResult{Updated: 1, Skipped: 2},
		},
		{
			name:      "ours is newer",
			oursAt:    base.Add(time.Hour),
			theirsAt:  base,
			wantPass:  "ours",
			wantCount: MergeResult{Skipped: 3},
		},
		{
			name:      "a tie keeps ours",
			oursAt:    base,
			theirsAt:  base,
			wantPass:  "ours",
			wantCount: MergeResult{Skipped: 3},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ours, theirs := twoCopies(t)
			id := idOf(t, ours, "GitHub")

			if err := ours.Update(id, Draft{Title: "GitHub", Password: "ours"}); err != nil {
				t.Fatal(err)
			}
			if err := theirs.Update(id, Draft{Title: "GitHub", Password: "theirs"}); err != nil {
				t.Fatal(err)
			}
			setModTime(t, ours, id, c.oursAt)
			setModTime(t, theirs, id, c.theirsAt)

			got, err := ours.Merge(theirs)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.wantCount {
				t.Errorf("MergeResult = %+v, want %+v", got, c.wantCount)
			}

			pw, err := ours.Reveal(id, FieldPassword)
			if err != nil {
				t.Fatal(err)
			}
			if pw != c.wantPass {
				t.Errorf("password = %q, want %q", pw, c.wantPass)
			}
		})
	}
}

func TestMergeAddsEntriesOnlyOnTheOtherSide(t *testing.T) {
	ours, theirs := twoCopies(t)

	if _, err := theirs.Add(Draft{Title: "Only Theirs", Password: "p", Username: "u"}); err != nil {
		t.Fatal(err)
	}

	res, err := ours.Merge(theirs)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("Added = %d, want 1", res.Added)
	}

	list, err := ours.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if m.Title == "Only Theirs" {
			found = true
			if m.GroupPath != "vaulty fixture" {
				t.Errorf("GroupPath = %q, want vaulty fixture", m.GroupPath)
			}
		}
	}
	if !found {
		t.Fatal("merged entry not present")
	}
	if !ours.Dirty() {
		t.Error("Dirty = false after a merge that added an entry")
	}
}

func TestMergeAddsEntryIntoAGroupWeDoNotHave(t *testing.T) {
	ours, theirs := twoCopies(t)

	gid, err := theirs.AddGroup(rootID(t, theirs), "Servers")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := theirs.Add(Draft{Title: "In Servers", Password: "p", GroupID: gid}); err != nil {
		t.Fatal(err)
	}

	res, err := ours.Merge(theirs)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Fatalf("Added = %d, want 1", res.Added)
	}

	groups, err := ours.Groups()
	if err != nil {
		t.Fatal(err)
	}
	var servers *GroupNode
	for i := range groups[0].Children {
		if groups[0].Children[i].Name == "Servers" {
			servers = &groups[0].Children[i]
		}
	}
	if servers == nil {
		t.Fatal("Servers group was not created by the merge")
	}
	if servers.Count != 1 {
		t.Errorf("Servers entry count = %d, want 1", servers.Count)
	}
}

func TestMergeAppliesTombstonesOnlyWhenOursIsOlder(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		oursAt      time.Time
		deletedAt   time.Time
		wantDeleted int
		wantPresent bool
	}{
		{"deleted after our edit", base, base.Add(time.Hour), 1, false},
		{"deleted before our edit", base.Add(time.Hour), base, 0, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ours, theirs := twoCopies(t)
			id := idOf(t, ours, "Bank")

			// Delete permanently on their side so a tombstone is recorded.
			if err := theirs.Delete(id); err != nil {
				t.Fatal(err)
			}
			if err := theirs.Delete(id); err != nil {
				t.Fatal(err)
			}
			theirs.db.Content.Root.DeletedObjects[0].DeletionTime.Time = c.deletedAt
			setModTime(t, ours, id, c.oursAt)

			res, err := ours.Merge(theirs)
			if err != nil {
				t.Fatal(err)
			}
			if res.Deleted != c.wantDeleted {
				t.Errorf("Deleted = %d, want %d", res.Deleted, c.wantDeleted)
			}

			_, err = ours.Detail(id)
			present := err == nil
			if present != c.wantPresent {
				t.Errorf("entry present = %v, want %v", present, c.wantPresent)
			}
		})
	}
}

// TestMergeJudgesEachTombstoneByItsOwnEntry covers two deletions in one
// group. Removing the first entry shifts the ones after it along the slice,
// so the second tombstone must not be weighed against whichever entry slid
// into its old place.
func TestMergeJudgesEachTombstoneByItsOwnEntry(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ours, theirs := twoCopies(t)
	gone := idOf(t, ours, "GitHub")
	kept := idOf(t, ours, "Bank")

	// Bank needs a neighbour after it, older than the deletions, to be the
	// entry that slides into its place.
	after, err := ours.Add(Draft{Title: "After Bank", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{gone, kept} {
		// Twice, so the second delete is permanent and records a tombstone.
		if err := theirs.Delete(id); err != nil {
			t.Fatal(err)
		}
		if err := theirs.Delete(id); err != nil {
			t.Fatal(err)
		}
	}
	for i := range theirs.db.Content.Root.DeletedObjects {
		theirs.db.Content.Root.DeletedObjects[i].DeletionTime.Time = base.Add(time.Hour)
	}
	setModTime(t, ours, gone, base)
	setModTime(t, ours, kept, base.Add(2*time.Hour))
	setModTime(t, ours, after, base)

	res, err := ours.Merge(theirs)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", res.Deleted)
	}
	if _, err := ours.Detail(gone); err == nil {
		t.Error("GitHub survived a tombstone newer than our copy")
	}
	if _, err := ours.Detail(kept); err != nil {
		t.Errorf("Bank was edited here after they deleted it and must stay: %v", err)
	}
}

func TestMergeIntoLockedVaultFails(t *testing.T) {
	ours, theirs := twoCopies(t)
	ours.Close()
	if _, err := ours.Merge(theirs); err == nil {
		t.Error("Merge into a locked vault succeeded, want an error")
	}
}

func rootID(t *testing.T, v *Vault) string {
	t.Helper()
	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	return groups[0].ID
}

// TestMergeAppliesTombstonesAlongsideNewGroups combines the two operations
// that fight over the same memory. The deleted entry lives in Work, a
// subgroup of the root; adding an entry to a brand new sibling group appends
// to the root's Groups slice, which moves Work. A tombstone pass running
// after the additions would hold a pointer to the old location.
func TestMergeAppliesTombstonesAlongsideNewGroups(t *testing.T) {
	ours, theirs := twoCopies(t)
	deletedID := idOf(t, ours, "Jira")

	gid, err := theirs.AddGroup(rootID(t, theirs), "Servers")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := theirs.Add(Draft{Title: "In Servers", Password: "p", GroupID: gid}); err != nil {
		t.Fatal(err)
	}
	if err := theirs.Delete(deletedID); err != nil {
		t.Fatal(err)
	}
	if err := theirs.Delete(deletedID); err != nil {
		t.Fatal(err)
	}
	for i := range theirs.db.Content.Root.DeletedObjects {
		theirs.db.Content.Root.DeletedObjects[i].DeletionTime.Time = time.Now().Add(time.Hour)
	}

	res, err := ours.Merge(theirs)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("Added = %d, want 1", res.Added)
	}
	if res.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", res.Deleted)
	}

	if _, err := ours.Detail(deletedID); err == nil {
		t.Error("tombstoned entry is still present")
	}
	list, err := ours.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		if m.Title == "Jira" {
			t.Error("Jira survived a tombstone that should have removed it")
		}
	}
	titles := map[string]bool{}
	for _, m := range list {
		titles[m.Title] = true
	}
	if !titles["In Servers"] {
		t.Error("merged entry In Servers missing")
	}
}

// TestMergeFromDiskUnblocksSave walks the whole conflict resolution: another
// writer changes the file, our Save is refused, the merge folds their work in
// and adopts their fingerprint, and the Save then succeeds.
func TestMergeFromDiskUnblocksSave(t *testing.T) {
	creds := Credentials{Password: fixturePassword}
	path := copyFixture(t, filepath.Join("testdata", "kdbx40-password.kdbx"))

	ours, err := Open(path, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer ours.Close()

	// Another client writes the same file behind our back.
	other, err := Open(path, creds)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Add(Draft{Title: "From The Other Client", Password: "theirs", Username: "them"}); err != nil {
		t.Fatal(err)
	}
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}
	other.Close()
	advanceModTime(t, path)

	if _, err := ours.Add(Draft{Title: "From Us", Password: "ours"}); err != nil {
		t.Fatal(err)
	}
	if err := ours.Save(); !errors.Is(err, ErrDiskChanged) {
		t.Fatalf("Save = %v, want ErrDiskChanged", err)
	}

	res, err := ours.MergeFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("Added = %d, want 1", res.Added)
	}
	if err := ours.Save(); err != nil {
		t.Fatalf("Save after MergeFromDisk: %v", err)
	}

	// Both clients' work must be in the file on disk.
	final, err := Open(path, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer final.Close()
	list, err := final.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]bool{}
	for _, m := range list {
		titles[m.Title] = true
	}
	for _, want := range []string{"From Us", "From The Other Client"} {
		if !titles[want] {
			t.Errorf("entry %q lost in the conflict resolution", want)
		}
	}
}
