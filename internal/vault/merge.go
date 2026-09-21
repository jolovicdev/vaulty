package vault

import (
	"github.com/tobischo/gokeepasslib/v3"
)

// MergeResult counts what a merge did.
type MergeResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Deleted int `json:"deleted"`
}

// Merge folds the other database into this one, entry by entry, keyed on
// entry UUID and resolved by last modification time. This is the "both sides
// changed" resolution for a vault kept in a sync folder.
//
// The rule is deliberately simple and only ever compares whole entries: the
// newer modification time wins, ties keep what is already here, an entry that
// exists only in other is added to the group with the matching name (created
// when absent), and an entry the other side recorded as deleted after our
// copy was last modified is removed. Field-level merging is not attempted,
// because two clients editing one entry's separate fields cannot be told
// apart from one client editing both.
func (v *Vault) Merge(other *Vault) (MergeResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	if err := v.checkOpen(); err != nil {
		return MergeResult{}, err
	}
	if err := other.checkOpen(); err != nil {
		return MergeResult{}, err
	}

	mine := map[string]*gokeepasslib.Entry{}
	myGroup := map[string]*gokeepasslib.Group{}
	v.walk(func(g *gokeepasslib.Group, _ string, _ bool) bool {
		for i := range g.Entries {
			id := entryID(&g.Entries[i])
			mine[id] = &g.Entries[i]
			myGroup[id] = g
		}
		return true
	})

	var res MergeResult
	type incoming struct {
		entry gokeepasslib.Entry
		path  string
	}
	var news []incoming

	other.walk(func(g *gokeepasslib.Group, path string, _ bool) bool {
		for i := range g.Entries {
			theirs := &g.Entries[i]
			id := entryID(theirs)
			ours, ok := mine[id]
			if !ok {
				news = append(news, incoming{entry: *theirs, path: path})
				continue
			}
			if modTime(theirs).After(modTime(ours)) {
				*ours = *theirs
				res.Updated++
			} else {
				res.Skipped++
			}
		}
		return true
	})

	// Tombstones are applied before the additions below, because creating a
	// group reallocates the slices that mine and myGroup point into. Only
	// entries that already existed can be tombstoned, so this order is also
	// the correct one semantically.
	//
	// Which tombstones apply is settled before any entry is removed. A
	// removal shifts the rest of its group along the slice, and a pointer in
	// mine taken before that reads a neighbour's modification time after it.
	type tombstone struct {
		id  string
		del gokeepasslib.DeletedObjectData
	}
	var apply []tombstone
	for _, del := range other.db.Content.Root.DeletedObjects {
		if del.DeletionTime == nil {
			continue
		}
		id, err := del.UUID.MarshalText()
		if err != nil {
			continue
		}
		ours, ok := mine[string(id)]
		if !ok || modTime(ours).After(del.DeletionTime.Time) {
			continue
		}
		apply = append(apply, tombstone{id: string(id), del: del})
	}
	for _, t := range apply {
		if err := removeEntry(myGroup[t.id], t.id); err == nil {
			v.db.Content.Root.DeletedObjects = append(v.db.Content.Root.DeletedObjects, t.del)
			res.Deleted++
		}
	}

	for _, in := range news {
		g, err := v.groupByPath(in.path)
		if err != nil {
			return res, err
		}
		g.Entries = append(g.Entries, in.entry)
		res.Added++
	}

	if res.Added+res.Updated+res.Deleted > 0 {
		v.dirty = true
	}
	return res, nil
}

// groupByPath finds a group by its slash separated path, creating any
// missing levels.
func (v *Vault) groupByPath(path string) (*gokeepasslib.Group, error) {
	var found *gokeepasslib.Group
	v.walk(func(g *gokeepasslib.Group, p string, _ bool) bool {
		if p == path {
			found = g
			return false
		}
		return true
	})
	if found != nil {
		return found, nil
	}
	if len(v.db.Content.Root.Groups) == 0 {
		return nil, ErrGroupNotFound
	}
	return v.createPath(path)
}

func (v *Vault) createPath(path string) (*gokeepasslib.Group, error) {
	names := splitPath(path)
	if len(names) == 0 {
		return &v.db.Content.Root.Groups[0], nil
	}
	// The first path element is a root group; match it or fall back to the
	// database's own root so a foreign tree still lands somewhere sensible.
	cur := &v.db.Content.Root.Groups[0]
	for i := range v.db.Content.Root.Groups {
		if v.db.Content.Root.Groups[i].Name == names[0] {
			cur = &v.db.Content.Root.Groups[i]
			break
		}
	}
	for _, name := range names[1:] {
		next := childByName(cur, name)
		if next == nil {
			g := gokeepasslib.NewGroup()
			g.Name = name
			cur.Groups = append(cur.Groups, g)
			next = &cur.Groups[len(cur.Groups)-1]
		}
		cur = next
	}
	return cur, nil
}

func childByName(g *gokeepasslib.Group, name string) *gokeepasslib.Group {
	for i := range g.Groups {
		if g.Groups[i].Name == name {
			return &g.Groups[i]
		}
	}
	return nil
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if i > start {
				out = append(out, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		out = append(out, path[start:])
	}
	return out
}

// AddGroup creates a subgroup under parent and returns its id.
func (v *Vault) AddGroup(parentID, name string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return "", err
	}
	parent := &v.db.Content.Root.Groups[0]
	if parentID != "" {
		g, err := v.findGroup(parentID)
		if err != nil {
			return "", err
		}
		parent = g
	}
	g := gokeepasslib.NewGroup()
	g.Name = name
	parent.Groups = append(parent.Groups, g)
	v.dirty = true
	return groupID(&parent.Groups[len(parent.Groups)-1]), nil
}
