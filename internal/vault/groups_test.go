package vault

import (
	"errors"
	"testing"
	"time"
)

func TestGroupRenameMoveAndDelete(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	groups, err := v.Groups()
	if err != nil {
		t.Fatal(err)
	}
	root := groups[0].ID

	a, err := v.AddGroup(root, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	b, err := v.AddGroup(root, "Beta")
	if err != nil {
		t.Fatal(err)
	}
	child, err := v.AddGroup(a, "Child")
	if err != nil {
		t.Fatal(err)
	}
	entryID, err := v.Add(Draft{Title: "Inside Child", Password: "p", GroupID: child})
	if err != nil {
		t.Fatal(err)
	}

	if err := v.RenameGroup(a, "Alpha Renamed"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if err := v.RenameGroup(a, "   "); err == nil {
		t.Error("RenameGroup accepted whitespace name")
	}

	// Moving a group into its own descendant or itself must fail.
	if err := v.MoveGroup(a, a); !errors.Is(err, ErrInvalidGroupMove) {
		t.Errorf("MoveGroup(self) = %v, want ErrInvalidGroupMove", err)
	}
	if err := v.MoveGroup(a, child); !errors.Is(err, ErrInvalidGroupMove) {
		t.Errorf("MoveGroup(descendant) = %v, want ErrInvalidGroupMove", err)
	}
	// Moving Alpha under Beta moves its subtree along with it.
	if err := v.MoveGroup(a, b); err != nil {
		t.Fatalf("MoveGroup: %v", err)
	}
	d, err := v.Detail(entryID)
	if err != nil {
		t.Fatal(err)
	}
	if d.GroupPath != "vaulty fixture/Beta/Alpha Renamed/Child" {
		t.Errorf("GroupPath = %q, want vaulty fixture/Beta/Alpha Renamed/Child", d.GroupPath)
	}

	// Root group is protected from deletion and moves.
	if err := v.DeleteGroup(root); !errors.Is(err, ErrProtectedGroup) {
		t.Errorf("DeleteGroup(root) = %v, want ErrProtectedGroup", err)
	}
	if err := v.MoveGroup(root, b); !errors.Is(err, ErrProtectedGroup) {
		t.Errorf("MoveGroup(root) = %v, want ErrProtectedGroup", err)
	}

	// Deleting Alpha recycles it and its descendant entry.
	if err := v.DeleteGroup(a); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	d, err = v.Detail(entryID)
	if err != nil {
		t.Fatal(err)
	}
	if !d.InRecycleBin {
		t.Error("entry inside deleted group is not marked InRecycleBin")
	}

	// Deleting again permanently removes the group and tombstones its entries.
	if err := v.DeleteGroup(a); err != nil {
		t.Fatalf("second DeleteGroup: %v", err)
	}
	if _, err := v.Detail(entryID); !errors.Is(err, ErrNotFound) {
		t.Errorf("entry survived permanent group deletion: %v", err)
	}
	if len(v.db.Content.Root.DeletedObjects) < 3 {
		t.Errorf("expected tombstones for group, child group and entry, got %d", len(v.db.Content.Root.DeletedObjects))
	}
}

func TestEntryExpiryAndDuplicate(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	past := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	future := time.Date(2099, 6, 15, 0, 0, 0, 0, time.UTC)

	id, err := v.Add(Draft{
		Title:      "API Key",
		Username:   "bot",
		Password:   "secret-1",
		Expires:    true,
		ExpiryTime: &past,
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := v.Detail(id)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Expires || !d.Expired || d.ExpiryTime == nil || !d.ExpiryTime.Equal(past) {
		t.Fatalf("expiry after Add = {Expires:%v Expired:%v ExpiryTime:%v}", d.Expires, d.Expired, d.ExpiryTime)
	}

	if err := v.Update(id, Draft{
		Title:      "API Key",
		Username:   "bot",
		Password:   "secret-1",
		Expires:    true,
		ExpiryTime: &future,
	}); err != nil {
		t.Fatal(err)
	}
	d, err = v.Detail(id)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Expires || d.Expired || d.ExpiryTime == nil || !d.ExpiryTime.Equal(future) {
		t.Fatalf("expiry after Update = {Expires:%v Expired:%v ExpiryTime:%v}", d.Expires, d.Expired, d.ExpiryTime)
	}

	dupID, err := v.Duplicate(id)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dupID == id {
		t.Fatal("Duplicate reused the original entry ID")
	}
	dup, err := v.Detail(dupID)
	if err != nil {
		t.Fatal(err)
	}
	if dup.Title != "API Key (copy)" {
		t.Errorf("dup.Title = %q, want API Key (copy)", dup.Title)
	}
	if dup.HistoryCount != 0 {
		t.Errorf("dup.HistoryCount = %d, want 0", dup.HistoryCount)
	}
	pw, err := v.Reveal(dupID, FieldPassword)
	if err != nil || pw != "secret-1" {
		t.Errorf("dup password = %q, %v, want secret-1", pw, err)
	}
}
