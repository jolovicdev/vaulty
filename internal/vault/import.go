package vault

import "github.com/tobischo/gokeepasslib/v3"

// Imported is one entry read from another password manager's export. Folder
// is the vault or folder path it had there, outermost first.
type Imported struct {
	Folder []string
	Draft  Draft
}

// Import adds a group called name under the root group and puts every entry
// in it, recreating each entry's folder path as subgroups. The batch goes in
// under one lock and marks the vault dirty once, so the save that follows
// writes it in a single pass. It returns the new group's id.
func (v *Vault) Import(name string, entries []Imported) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return "", err
	}
	if len(v.db.Content.Root.Groups) == 0 {
		return "", ErrGroupNotFound
	}

	top := gokeepasslib.NewGroup()
	top.Name = name
	for _, im := range entries {
		g := &top
		for _, folder := range im.Folder {
			next := childByName(g, folder)
			if next == nil {
				sub := gokeepasslib.NewGroup()
				sub.Name = folder
				g.Groups = append(g.Groups, sub)
				next = &g.Groups[len(g.Groups)-1]
			}
			g = next
		}
		e := gokeepasslib.NewEntry()
		applyDraft(&e, im.Draft)
		g.Entries = append(g.Entries, e)
	}

	root := &v.db.Content.Root.Groups[0]
	root.Groups = append(root.Groups, top)
	v.dirty = true
	return groupID(&top), nil
}
