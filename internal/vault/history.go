package vault

import "github.com/tobischo/gokeepasslib/v3"

// History lists an entry's past revisions, newest last, without secrets.
func (v *Vault) History(id string) ([]HistoryVersion, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, _, _, _, err := v.findEntry(id)
	if err != nil {
		return nil, err
	}

	out := []HistoryVersion{}
	for i := range e.Histories {
		for j := range e.Histories[i].Entries {
			h := &e.Histories[i].Entries[j]
			out = append(out, HistoryVersion{
				Index:    len(out),
				Title:    value(h, FieldTitle),
				Username: value(h, FieldUserName),
				URL:      value(h, FieldURL),
				Modified: modTime(h),
			})
		}
	}
	return out, nil
}

// RevealHistory returns one field of one past revision.
func (v *Vault) RevealHistory(id string, index int, field string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	h, err := v.historyAt(id, index)
	if err != nil {
		return "", err
	}
	val := h.Get(field)
	if val == nil {
		return "", ErrNotFound
	}
	return val.Value.Content, nil
}

func (v *Vault) historyAt(id string, index int) (*gokeepasslib.Entry, error) {
	e, _, _, _, err := v.findEntry(id)
	if err != nil {
		return nil, err
	}
	n := 0
	for i := range e.Histories {
		for j := range e.Histories[i].Entries {
			if n == index {
				return &e.Histories[i].Entries[j], nil
			}
			n++
		}
	}
	return nil, ErrNotFound
}

// RestoreVersion copies a past revision's fields back onto the live entry.
// The state being replaced is pushed onto the history first, so a restore is
// itself undoable.
func (v *Vault) RestoreVersion(id string, index int) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	h, err := v.historyAt(id, index)
	if err != nil {
		return err
	}
	restored := make([]gokeepasslib.ValueData, len(h.Values))
	copy(restored, h.Values)
	tags := h.Tags

	e, _, _, _, err := v.findEntry(id)
	if err != nil {
		return err
	}
	v.archive(e)
	e.Values = restored
	e.Tags = tags
	touch(e)
	v.dirty = true
	return nil
}
