package vault

import (
	"testing"
	"time"

	"github.com/jolovicdev/vaulty/internal/strength"
)

func TestAuditFindsWeakReusedAndExpiringEntries(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-48 * time.Hour)
	soon := now.Add(7 * 24 * time.Hour)
	far := now.Add(90 * 24 * time.Hour)

	if _, err := v.Add(Draft{Title: "WeakOne", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Add(Draft{Title: "ReuseA", Password: "8Kq#mV2b$TrW7nHdL!pXz"}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Add(Draft{Title: "ReuseB", Password: "8Kq#mV2b$TrW7nHdL!pXz"}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Add(Draft{Title: "ExpiredKey", Password: "zX9@qW4#mN7$pL2&kV8*", Expires: true, ExpiryTime: &past}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Add(Draft{Title: "SoonKey", Password: "aB3!cD5@eF7#gH9$jK2%", Expires: true, ExpiryTime: &soon}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Add(Draft{Title: "FarKey", Password: "mN4^pQ6&rS8*tU2!vW9@", Expires: true, ExpiryTime: &far}); err != nil {
		t.Fatal(err)
	}
	// Recycled entries must not pollute the audit.
	recycledID, err := v.Add(Draft{Title: "TrashedWeak", Password: "123456"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Delete(recycledID); err != nil {
		t.Fatal(err)
	}

	dict := strength.NewDictionary([]string{"password", "dragon"})
	rep, err := v.Audit(dict, now)
	if err != nil {
		t.Fatal(err)
	}

	foundWeak := false
	for _, w := range rep.Weak {
		if w.Entry.Title == "WeakOne" {
			foundWeak = true
		}
		if w.Entry.Title == "TrashedWeak" {
			t.Error("recycled entry appeared in weak findings")
		}
	}
	if !foundWeak {
		t.Errorf("WeakOne missing from Weak: %+v", rep.Weak)
	}

	foundReused := false
	for _, grp := range rep.Reused {
		if len(grp.Entries) == 2 && grp.Entries[0].Title == "ReuseA" && grp.Entries[1].Title == "ReuseB" {
			foundReused = true
		}
	}
	if !foundReused {
		t.Errorf("ReuseA/ReuseB missing from Reused: %+v", rep.Reused)
	}

	if len(rep.Expired) != 1 || rep.Expired[0].Title != "ExpiredKey" {
		t.Errorf("Expired = %+v, want [ExpiredKey]", rep.Expired)
	}
	if len(rep.ExpiringSoon) != 1 || rep.ExpiringSoon[0].Title != "SoonKey" {
		t.Errorf("ExpiringSoon = %+v, want [SoonKey]", rep.ExpiringSoon)
	}
}
