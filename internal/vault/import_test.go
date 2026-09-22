package vault

import (
	"testing"
)

func TestImportBuildsTheFolderTreeUnderOneGroup(t *testing.T) {
	v := openWritable(t, "kdbx40-password.kdbx")

	id, err := v.Import("Imported from Test", []Imported{
		{Folder: []string{"Work", "Servers"}, Draft: Draft{Title: "db", Password: "p1"}},
		{Folder: []string{"Work", "Servers"}, Draft: Draft{
			Title:           "api",
			Custom:          map[string]string{"Token": "tok-1"},
			ProtectedCustom: []string{"Token"},
		}},
		{Draft: Draft{Title: "loose"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Dirty() {
		t.Error("Dirty = false after Import")
	}
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(v.Path(), Credentials{Password: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	groups, err := reopened.Groups()
	if err != nil {
		t.Fatal(err)
	}
	var top *GroupNode
	for i, c := range groups[0].Children {
		if c.ID == id {
			top = &groups[0].Children[i]
		}
	}
	if top == nil {
		t.Fatal("the import group is not under the root group")
	}
	if top.Name != "Imported from Test" || top.Count != 1 {
		t.Errorf("import group = %q holding %d, want %q holding 1", top.Name, top.Count, "Imported from Test")
	}
	if len(top.Children) != 1 || top.Children[0].Name != "Work" {
		t.Fatalf("import group children = %+v, want one Work group", top.Children)
	}
	servers := top.Children[0].Children
	if len(servers) != 1 || servers[0].Name != "Servers" || servers[0].Count != 2 {
		t.Fatalf("Work children = %+v, want one Servers group holding 2", servers)
	}

	d, err := reopened.Detail(idOf(t, reopened, "api"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Custom) != 1 || !d.Custom[0].Protected || d.Custom[0].Value != "" {
		t.Errorf("Custom = %+v, want Token protected and withheld", d.Custom)
	}
}
