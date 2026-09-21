package vault

import (
	"github.com/tobischo/gokeepasslib/v3"
	w "github.com/tobischo/gokeepasslib/v3/wrappers"
)

// recycleBinName matches what KeePassXC creates, so a bin made here is the
// same bin it shows.
const recycleBinName = "Recycle Bin"

// recycleBinIcon is the trash can in the standard KeePass icon set.
const recycleBinIcon = 43

// ensureRecycleBin returns the bin group, creating it at the root when the
// database has recycling enabled but no bin yet.
func (v *Vault) ensureRecycleBin() (*gokeepasslib.Group, error) {
	if v.db.Content.Meta == nil || !v.db.Content.Meta.RecycleBinEnabled.Bool {
		return nil, ErrNoRecycleBin
	}
	if id := v.recycleBinUUID(); !id.IsZero() {
		var found *gokeepasslib.Group
		v.walk(func(g *gokeepasslib.Group, _ string, _ bool) bool {
			if g.UUID.Compare(id) {
				found = g
				return false
			}
			return true
		})
		if found != nil {
			return found, nil
		}
	}
	if len(v.db.Content.Root.Groups) == 0 {
		return nil, ErrGroupNotFound
	}

	bin := gokeepasslib.NewGroup()
	bin.Name = recycleBinName
	bin.IconID = recycleBinIcon
	bin.EnableAutoType = w.NewNullableBoolWrapper(false)
	bin.EnableSearching = w.NewNullableBoolWrapper(false)

	root := &v.db.Content.Root.Groups[0]
	root.Groups = append(root.Groups, bin)
	added := &root.Groups[len(root.Groups)-1]

	changed := w.Now()
	v.db.Content.Meta.RecycleBinUUID = added.UUID
	v.db.Content.Meta.RecycleBinChanged = &changed
	return added, nil
}

// Delete moves an entry to the recycle bin, or removes it outright when it is
// already in the bin or the database has recycling disabled. A permanent
// removal records a DeletedObject so other KeePass clients replicate it.
func (v *Vault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, owner, _, inBin, err := v.findEntry(id)
	if err != nil {
		return err
	}

	if inBin || v.db.Content.Meta == nil || !v.db.Content.Meta.RecycleBinEnabled.Bool {
		uuid := e.UUID
		if err := removeEntry(owner, id); err != nil {
			return err
		}
		now := w.Now()
		v.db.Content.Root.DeletedObjects = append(v.db.Content.Root.DeletedObjects,
			gokeepasslib.DeletedObjectData{UUID: uuid, DeletionTime: &now})
		v.dirty = true
		return nil
	}

	bin, err := v.ensureRecycleBin()
	if err != nil {
		return err
	}
	// findEntry's pointer can dangle once ensureRecycleBin appends to the
	// root slice, so look the entry up again before moving it.
	e, owner, _, _, err = v.findEntry(id)
	if err != nil {
		return err
	}
	moved := *e
	if err := removeEntry(owner, id); err != nil {
		return err
	}
	loc := w.Now()
	moved.Times.LocationChanged = &loc
	bin.Entries = append(bin.Entries, moved)
	v.dirty = true
	return nil
}

// Restore moves an entry out of the recycle bin into the given group, or into
// the first root group when none is named.
func (v *Vault) Restore(id, groupTarget string) error {
	v.mu.RLock()
	_, _, _, inBin, err := v.findEntry(id)
	v.mu.RUnlock()
	if err != nil {
		return err
	}
	if !inBin {
		return nil
	}
	if groupTarget == "" {
		v.mu.RLock()
		if len(v.db.Content.Root.Groups) == 0 {
			v.mu.RUnlock()
			return ErrGroupNotFound
		}
		groupTarget = groupID(&v.db.Content.Root.Groups[0])
		v.mu.RUnlock()
	}
	return v.Move(id, groupTarget)
}

// EmptyRecycleBin permanently removes every entry in the bin.
func (v *Vault) EmptyRecycleBin() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return err
	}
	id := v.recycleBinUUID()
	if id.IsZero() {
		return ErrNoRecycleBin
	}

	now := w.Now()
	var purge func(g *gokeepasslib.Group)
	purge = func(g *gokeepasslib.Group) {
		for i := range g.Entries {
			v.db.Content.Root.DeletedObjects = append(v.db.Content.Root.DeletedObjects,
				gokeepasslib.DeletedObjectData{UUID: g.Entries[i].UUID, DeletionTime: &now})
		}
		for i := range g.Groups {
			purge(&g.Groups[i])
		}
		g.Entries = nil
		g.Groups = nil
	}

	v.walk(func(g *gokeepasslib.Group, _ string, _ bool) bool {
		if g.UUID.Compare(id) {
			purge(g)
			return false
		}
		return true
	})
	v.dirty = true
	return nil
}
