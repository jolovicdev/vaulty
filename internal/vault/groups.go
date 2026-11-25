package vault

import (
	"errors"
	"strings"

	"github.com/tobischo/gokeepasslib/v3"
	w "github.com/tobischo/gokeepasslib/v3/wrappers"
)

var (
	// ErrProtectedGroup is returned when an operation tries to rename, move
	// or delete a root group or the recycle bin.
	ErrProtectedGroup = errors.New("this group cannot be modified")
	// ErrInvalidGroupMove is returned when moving a group into itself, a
	// descendant, or the recycle bin.
	ErrInvalidGroupMove = errors.New("group cannot be moved there")
)

// RenameGroup changes a group's name. The recycle bin cannot be renamed.
func (v *Vault) RenameGroup(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("group name cannot be empty")
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return err
	}
	g, err := v.findGroup(id)
	if err != nil {
		return err
	}
	bin := v.recycleBinUUID()
	if !bin.IsZero() && g.UUID.Compare(bin) {
		return ErrProtectedGroup
	}

	g.Name = name
	now := w.Now()
	g.Times.LastModificationTime = &now
	v.dirty = true
	return nil
}

// DeleteGroup moves a subgroup into the recycle bin, or removes it permanently
// when it is already in the bin or recycling is disabled. Root groups and the
// recycle bin itself cannot be deleted.
func (v *Vault) DeleteGroup(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return err
	}

	g, parent, idx, inBin, err := v.findGroupWithParent(id)
	if err != nil {
		return err
	}
	bin := v.recycleBinUUID()
	if parent == nil || (!bin.IsZero() && g.UUID.Compare(bin)) {
		return ErrProtectedGroup
	}

	if inBin || v.db.Content.Meta == nil || !v.db.Content.Meta.RecycleBinEnabled.Bool {
		removed := parent.Groups[idx]
		parent.Groups = append(parent.Groups[:idx], parent.Groups[idx+1:]...)
		now := w.Now()
		v.recordGroupTombstones(&removed, &now)
		v.dirty = true
		return nil
	}

	recycle, err := v.ensureRecycleBin()
	if err != nil {
		return err
	}
	// Re-resolve after ensureRecycleBin may have reallocated root.Groups[0].Groups.
	g, parent, idx, _, err = v.findGroupWithParent(id)
	if err != nil {
		return err
	}
	if parent == nil || g == recycle {
		return ErrProtectedGroup
	}
	moved := *g
	parent.Groups = append(parent.Groups[:idx], parent.Groups[idx+1:]...)
	loc := w.Now()
	moved.Times.LocationChanged = &loc
	recycle.Groups = append(recycle.Groups, moved)
	v.dirty = true
	return nil
}

// MoveGroup relocates a subgroup under another parent group. Root groups and
// the recycle bin cannot be moved, and a group cannot be moved into itself,
// one of its descendants, or the recycle bin.
func (v *Vault) MoveGroup(id, targetParentID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return err
	}

	g, parent, idx, _, err := v.findGroupWithParent(id)
	if err != nil {
		return err
	}
	bin := v.recycleBinUUID()
	if parent == nil || (!bin.IsZero() && g.UUID.Compare(bin)) {
		return ErrProtectedGroup
	}
	if targetParentID == "" {
		if len(v.db.Content.Root.Groups) == 0 {
			return ErrGroupNotFound
		}
		targetParentID = groupID(&v.db.Content.Root.Groups[0])
	}
	if groupID(parent) == targetParentID {
		return nil
	}
	if id == targetParentID || groupContains(g, targetParentID) {
		return ErrInvalidGroupMove
	}
	_, _, _, dstInBin, err := v.findGroupWithParent(targetParentID)
	if err != nil {
		return err
	}
	if dstInBin {
		return ErrInvalidGroupMove
	}

	moved := *g
	parent.Groups = append(parent.Groups[:idx], parent.Groups[idx+1:]...)
	// Re-resolve dst after removing from parent in case dst was a sibling
	// whose address shifted inside the same slice.
	dst, err := v.findGroup(targetParentID)
	if err != nil {
		return err
	}
	loc := w.Now()
	moved.Times.LocationChanged = &loc
	dst.Groups = append(dst.Groups, moved)
	v.dirty = true
	return nil
}

func (v *Vault) findGroupWithParent(id string) (found, parent *gokeepasslib.Group, index int, inBin bool, err error) {
	bin := v.recycleBinUUID()
	var rec func(groups []gokeepasslib.Group, p *gokeepasslib.Group, underBin bool) bool
	rec = func(groups []gokeepasslib.Group, p *gokeepasslib.Group, underBin bool) bool {
		for i := range groups {
			g := &groups[i]
			under := underBin || (!bin.IsZero() && g.UUID.Compare(bin))
			if groupID(g) == id {
				found, parent, index, inBin = g, p, i, under
				return false
			}
			if !rec(g.Groups, g, under) {
				return false
			}
		}
		return true
	}
	rec(v.db.Content.Root.Groups, nil, false)
	if found == nil {
		return nil, nil, -1, false, ErrGroupNotFound
	}
	return found, parent, index, inBin, nil
}

func groupContains(root *gokeepasslib.Group, id string) bool {
	for i := range root.Groups {
		if groupID(&root.Groups[i]) == id || groupContains(&root.Groups[i], id) {
			return true
		}
	}
	return false
}

func (v *Vault) recordGroupTombstones(g *gokeepasslib.Group, now *w.TimeWrapper) {
	for i := range g.Entries {
		v.db.Content.Root.DeletedObjects = append(v.db.Content.Root.DeletedObjects,
			gokeepasslib.DeletedObjectData{UUID: g.Entries[i].UUID, DeletionTime: now})
	}
	for i := range g.Groups {
		v.recordGroupTombstones(&g.Groups[i], now)
	}
	v.db.Content.Root.DeletedObjects = append(v.db.Content.Root.DeletedObjects,
		gokeepasslib.DeletedObjectData{UUID: g.UUID, DeletionTime: now})
}
