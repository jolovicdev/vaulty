package vault

import (
	"errors"
	"slices"
	"time"

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
// exists only in other is added to the group with the matching path (created
// when absent, or to the recycle bin when recycled in other) unless we
// permanently deleted it more recently, and an entry the other side recorded
// as deleted after our copy was last modified is removed. Field-level merging
// is not attempted, because two clients editing one entry's separate fields
// cannot be told apart from one client editing both.
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
	myPath := map[string][]string{}
	myInBin := map[string]bool{}
	v.walkSegments(func(g *gokeepasslib.Group, segs []string, inBin bool) bool {
		for i := range g.Entries {
			id := entryID(&g.Entries[i])
			mine[id] = &g.Entries[i]
			myGroup[id] = g
			myPath[id] = slices.Clone(segs)
			myInBin[id] = inBin
		}
		return true
	})

	myDeleted := map[string]time.Time{}
	for _, del := range v.db.Content.Root.DeletedObjects {
		if del.DeletionTime == nil {
			continue
		}
		id, err := del.UUID.MarshalText()
		if err != nil {
			continue
		}
		key := string(id)
		if prev, ok := myDeleted[key]; !ok || del.DeletionTime.Time.After(prev) {
			myDeleted[key] = del.DeletionTime.Time
		}
	}

	var res MergeResult
	type incoming struct {
		entry gokeepasslib.Entry
		segs  []string
		inBin bool
	}
	type relocation struct {
		id    string
		segs  []string
		inBin bool
	}
	var news []incoming
	var moves []relocation

	other.walkSegments(func(g *gokeepasslib.Group, segs []string, inBin bool) bool {
		for i := range g.Entries {
			theirs := &g.Entries[i]
			id := entryID(theirs)
			ours, ok := mine[id]
			if !ok {
				if delAt, deleted := myDeleted[id]; deleted && !modTime(theirs).After(delAt) {
					continue
				}
				news = append(news, incoming{
					entry: cloneEntry(theirs),
					segs:  slices.Clone(segs),
					inBin: inBin,
				})
				continue
			}
			updated := false
			if modTime(theirs).After(modTime(ours)) {
				*ours = cloneEntry(theirs)
				updated = true
			}
			if locTime(theirs).After(locTime(ours)) &&
				(inBin != myInBin[id] || !slices.Equal(segs, myPath[id])) {
				moves = append(moves, relocation{
					id:    id,
					segs:  slices.Clone(segs),
					inBin: inBin,
				})
				updated = true
			}
			if updated {
				res.Updated++
			} else {
				res.Skipped++
			}
		}
		return true
	})

	// Tombstones are applied before the additions and moves below, because
	// creating a group reallocates the slices that mine and myGroup point
	// into. Only entries that already existed can be tombstoned, so this
	// order is also the correct one semantically.
	//
	// Which tombstones apply is settled before any entry is removed. A
	// removal shifts the rest of its group along the slice, and a pointer in
	// mine taken before that reads a neighbour's modification time after it.
	type tombstone struct {
		id  string
		del gokeepasslib.DeletedObjectData
	}
	var apply []tombstone
	tombstoned := map[string]bool{}
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
		tombstoned[string(id)] = true
	}
	for _, t := range apply {
		if err := removeEntry(myGroup[t.id], t.id); err == nil {
			v.db.Content.Root.DeletedObjects = append(v.db.Content.Root.DeletedObjects, t.del)
			res.Deleted++
		}
	}

	for _, mv := range moves {
		if tombstoned[mv.id] {
			continue
		}
		dst, err := v.targetGroupForMerge(mv.segs, mv.inBin)
		if err != nil {
			if errors.Is(err, ErrNoRecycleBin) {
				continue
			}
			return res, err
		}
		e, owner, _, _, err := v.findEntry(mv.id)
		if err != nil {
			continue
		}
		if err := v.move(e, owner, dst); err != nil {
			return res, err
		}
	}

	for _, in := range news {
		g, err := v.targetGroupForMerge(in.segs, in.inBin)
		if err != nil {
			if errors.Is(err, ErrNoRecycleBin) {
				continue
			}
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

func locTime(e *gokeepasslib.Entry) time.Time {
	if e.Times.LocationChanged != nil {
		return e.Times.LocationChanged.Time
	}
	return time.Time{}
}

// cloneEntry deep-copies an entry including its history so merging two open
// Vault values never shares backing arrays between them.
func cloneEntry(e *gokeepasslib.Entry) gokeepasslib.Entry {
	out := snapshot(e)
	if len(e.Histories) > 0 {
		out.Histories = make([]gokeepasslib.History, len(e.Histories))
		for i := range e.Histories {
			src := e.Histories[i].Entries
			dst := make([]gokeepasslib.Entry, len(src))
			for j := range src {
				dst[j] = snapshot(&src[j])
			}
			out.Histories[i].Entries = dst
		}
	}
	return out
}

// walkSegments visits every group depth first, passing each level's name as a
// slice so group names containing "/" are preserved without ambiguity.
func (v *Vault) walkSegments(fn func(g *gokeepasslib.Group, segs []string, inBin bool) bool) {
	bin := v.recycleBinUUID()
	var rec func(groups []gokeepasslib.Group, prefix []string, inBin bool) bool
	rec = func(groups []gokeepasslib.Group, prefix []string, inBin bool) bool {
		for i := range groups {
			g := &groups[i]
			segs := make([]string, len(prefix)+1)
			copy(segs, prefix)
			segs[len(prefix)] = g.Name
			under := inBin || (!bin.IsZero() && g.UUID.Compare(bin))
			if !fn(g, segs, under) {
				return false
			}
			if !rec(g.Groups, segs, under) {
				return false
			}
		}
		return true
	}
	rec(v.db.Content.Root.Groups, nil, false)
}

func (v *Vault) targetGroupForMerge(segs []string, inBin bool) (*gokeepasslib.Group, error) {
	if inBin {
		return v.ensureRecycleBin()
	}
	return v.groupBySegments(segs)
}

// groupBySegments finds a group by its path segments, creating any missing
// levels under the root group.
func (v *Vault) groupBySegments(segs []string) (*gokeepasslib.Group, error) {
	var found *gokeepasslib.Group
	v.walkSegments(func(g *gokeepasslib.Group, cur []string, inBin bool) bool {
		if !inBin && slices.Equal(cur, segs) {
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
	return v.createSegments(segs)
}

func (v *Vault) createSegments(names []string) (*gokeepasslib.Group, error) {
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

// AddGroup creates a subgroup under parent and returns its id.
func (v *Vault) AddGroup(parentID, name string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return "", err
	}
	if len(v.db.Content.Root.Groups) == 0 {
		return "", ErrGroupNotFound
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
