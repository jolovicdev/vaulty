package vault

import (
	"sort"
	"strings"
	"time"

	"github.com/tobischo/gokeepasslib/v3"
	w "github.com/tobischo/gokeepasslib/v3/wrappers"
)

func entryID(e *gokeepasslib.Entry) string {
	b, err := e.UUID.MarshalText()
	if err != nil {
		return ""
	}
	return string(b)
}

func groupID(g *gokeepasslib.Group) string {
	b, err := g.UUID.MarshalText()
	if err != nil {
		return ""
	}
	return string(b)
}

func splitTags(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinTags(tags []string) string {
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			clean = append(clean, t)
		}
	}
	return strings.Join(clean, ";")
}

func sortStrings(s []string) {
	sort.Slice(s, func(i, j int) bool { return strings.ToLower(s[i]) < strings.ToLower(s[j]) })
}

func value(e *gokeepasslib.Entry, key string) string {
	if v := e.Get(key); v != nil {
		return v.Value.Content
	}
	return ""
}

// totpSeed returns the otpauth URI or raw seed, whichever KeePassXC wrote.
// KeePassXC uses the "otp" field for an otpauth:// URI and the pair
// "TOTP Seed"/"TOTP Settings" for its older format.
func totpSeed(e *gokeepasslib.Entry) string {
	if s := value(e, FieldTOTPSeed); s != "" {
		return s
	}
	return value(e, "TOTP Seed")
}

func modTime(e *gokeepasslib.Entry) time.Time {
	if e.Times.LastModificationTime != nil {
		return e.Times.LastModificationTime.Time
	}
	return time.Time{}
}

func expired(e *gokeepasslib.Entry) bool {
	if !e.Times.Expires.Bool || e.Times.ExpiryTime == nil {
		return false
	}
	return e.Times.ExpiryTime.Time.Before(time.Now())
}

func (v *Vault) meta(e *gokeepasslib.Entry, g *gokeepasslib.Group, path string, inBin bool) Meta {
	m := Meta{
		ID:           entryID(e),
		Title:        value(e, FieldTitle),
		Username:     value(e, FieldUserName),
		URL:          value(e, FieldURL),
		Tags:         splitTags(e.Tags),
		GroupID:      groupID(g),
		GroupPath:    path,
		Modified:     modTime(e),
		HasTOTP:      totpSeed(e) != "",
		HasNotes:     value(e, FieldNotes) != "",
		Expired:      expired(e),
		InRecycleBin: inBin,
	}
	if e.Times.CreationTime != nil {
		m.Created = e.Times.CreationTime.Time
	}
	return m
}

// ListOptions narrows what List returns.
type ListOptions struct {
	GroupID           string `json:"groupId"`
	Tag               string `json:"tag"`
	IncludeRecycleBin bool   `json:"includeRecycleBin"`
}

// List returns entry metadata only. No password, notes or protected custom
// field ever appears in the result.
func (v *Vault) List(opts ListOptions) ([]Meta, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.listLocked(opts)
}

// listLocked is List with the caller already holding v.mu.
func (v *Vault) listLocked(opts ListOptions) ([]Meta, error) {
	if err := v.checkOpen(); err != nil {
		return nil, err
	}

	out := []Meta{}
	v.walk(func(g *gokeepasslib.Group, path string, inBin bool) bool {
		if inBin && !opts.IncludeRecycleBin {
			return true
		}
		if opts.GroupID != "" && groupID(g) != opts.GroupID {
			return true
		}
		for i := range g.Entries {
			e := &g.Entries[i]
			if opts.Tag != "" && !hasTag(e.Tags, opts.Tag) {
				continue
			}
			out = append(out, v.meta(e, g, path, inBin))
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out, nil
}

func hasTag(raw, tag string) bool {
	for _, t := range splitTags(raw) {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// Detail returns everything about an entry except its secrets.
func (v *Vault) Detail(id string) (Detail, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, g, path, inBin, err := v.findEntry(id)
	if err != nil {
		return Detail{}, err
	}

	d := Detail{Meta: v.meta(e, g, path, inBin), Custom: []CustomField{}}
	d.PasswordSet = value(e, FieldPassword) != ""
	for i := range e.Histories {
		d.HistoryCount += len(e.Histories[i].Entries)
	}
	for i := range e.Values {
		val := &e.Values[i]
		if IsStandardField(val.Key) {
			continue
		}
		f := CustomField{Key: val.Key, Protected: val.Value.Protected.Bool}
		if !f.Protected {
			f.Value = val.Value.Content
		}
		d.Custom = append(d.Custom, f)
	}
	sort.Slice(d.Custom, func(i, j int) bool { return d.Custom[i].Key < d.Custom[j].Key })
	return d, nil
}

// IsStandardField reports whether key is one of the fields every entry has,
// which a custom field cannot be named after.
func IsStandardField(key string) bool {
	switch key {
	case FieldTitle, FieldUserName, FieldPassword, FieldURL, FieldNotes, FieldTOTPSeed:
		return true
	}
	return false
}

// Reveal returns the value of exactly one field of exactly one entry. It is
// the only way a secret leaves this package.
func (v *Vault) Reveal(id, field string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, _, _, _, err := v.findEntry(id)
	if err != nil {
		return "", err
	}
	if field == "" {
		return "", ErrReadOnlyField
	}
	val := e.Get(field)
	if val == nil {
		return "", ErrNotFound
	}
	return val.Value.Content, nil
}

// TOTPSeed returns the stored otpauth URI or seed for an entry.
func (v *Vault) TOTPSeed(id string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, _, _, _, err := v.findEntry(id)
	if err != nil {
		return "", err
	}
	s := totpSeed(e)
	if s == "" {
		return "", ErrNotFound
	}
	if settings := value(e, "TOTP Settings"); settings != "" && !strings.HasPrefix(s, "otpauth://") {
		return s + "|" + settings, nil
	}
	return s, nil
}

func protectedValue(key, content string, protect bool) gokeepasslib.ValueData {
	return gokeepasslib.ValueData{
		Key:   key,
		Value: gokeepasslib.V{Content: content, Protected: w.NewBoolWrapper(protect)},
	}
}

func applyDraft(e *gokeepasslib.Entry, d Draft) {
	e.Values = make([]gokeepasslib.ValueData, 0, len(d.Custom)+6)
	e.Values = append(e.Values,
		protectedValue(FieldTitle, d.Title, false),
		protectedValue(FieldUserName, d.Username, false),
		protectedValue(FieldPassword, d.Password, true),
		protectedValue(FieldURL, d.URL, false),
		protectedValue(FieldNotes, d.Notes, false),
	)
	if d.TOTPSeed != "" {
		e.Values = append(e.Values, protectedValue(FieldTOTPSeed, d.TOTPSeed, true))
	}

	protect := map[string]bool{}
	for _, k := range d.ProtectedCustom {
		protect[k] = true
	}
	keys := make([]string, 0, len(d.Custom))
	for k := range d.Custom {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if IsStandardField(k) || (d.RemoveTOTP && isLegacyTOTPField(k)) {
			continue
		}
		e.Values = append(e.Values, protectedValue(k, d.Custom[k], protect[k]))
	}
	e.Tags = joinTags(d.Tags)
}

// isLegacyTOTPField names the pair of custom fields a seed lived in before
// the otp field. The editor sends them back like any other custom field, so
// removing a one-time code has to leave them out.
func isLegacyTOTPField(key string) bool {
	return key == "TOTP Seed" || key == "TOTP Settings"
}

// Add creates an entry in the group named by the draft, or in the first root
// group when the draft names none. It returns the new entry's id.
func (v *Vault) Add(d Draft) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return "", err
	}

	target := d.GroupID
	if target == "" {
		if len(v.db.Content.Root.Groups) == 0 {
			return "", ErrGroupNotFound
		}
		target = groupID(&v.db.Content.Root.Groups[0])
	}
	g, err := v.findGroup(target)
	if err != nil {
		return "", err
	}

	e := gokeepasslib.NewEntry()
	applyDraft(&e, d)
	g.Entries = append(g.Entries, e)
	v.dirty = true
	return entryID(&e), nil
}

// Update replaces an entry's fields, pushing the previous state onto the
// entry's KDBX history first. A draft that names a different group also
// moves the entry.
func (v *Vault) Update(id string, d Draft) error {
	v.mu.Lock()
	e, owner, _, _, err := v.findEntry(id)
	if err != nil {
		v.mu.Unlock()
		return err
	}

	// The editor never loads the stored seed, so an empty one means
	// unchanged. applyDraft rebuilds the values from the draft alone and
	// would drop it. Taking the code off is therefore its own flag.
	switch {
	case d.RemoveTOTP:
		d.TOTPSeed = ""
	case d.TOTPSeed == "":
		d.TOTPSeed = value(e, FieldTOTPSeed)
	}

	v.archive(e)
	applyDraft(e, d)
	touch(e)
	v.dirty = true

	moveTo := ""
	if d.GroupID != "" && d.GroupID != groupID(owner) {
		moveTo = d.GroupID
	}
	v.mu.Unlock()

	if moveTo == "" {
		return nil
	}
	return v.Move(id, moveTo)
}

// trimHistory honours the database's HistoryMaxItems setting, which is what
// KeePassXC enforces on its side.
func (v *Vault) trimHistory(e *gokeepasslib.Entry) {
	if len(e.Histories) == 0 || v.db.Content.Meta == nil {
		return
	}
	max := int(v.db.Content.Meta.HistoryMaxItems)
	if max <= 0 {
		return
	}
	h := &e.Histories[0]
	if len(h.Entries) > max {
		h.Entries = h.Entries[len(h.Entries)-max:]
	}
}

func touch(e *gokeepasslib.Entry) {
	now := w.Now()
	e.Times.LastModificationTime = &now
	access := w.Now()
	e.Times.LastAccessTime = &access
}

// Move relocates an entry into another group.
func (v *Vault) Move(id, groupTarget string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, owner, _, _, err := v.findEntry(id)
	if err != nil {
		return err
	}
	dst, err := v.findGroup(groupTarget)
	if err != nil {
		return err
	}
	if owner == dst {
		return nil
	}

	moved := *e
	if err := removeEntry(owner, id); err != nil {
		return err
	}
	loc := w.Now()
	moved.Times.LocationChanged = &loc
	dst.Entries = append(dst.Entries, moved)
	v.dirty = true
	return nil
}

func removeEntry(g *gokeepasslib.Group, id string) error {
	for i := range g.Entries {
		if entryID(&g.Entries[i]) == id {
			g.Entries = append(g.Entries[:i], g.Entries[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// snapshot copies an entry for the history list. It keeps the UUID, unlike
// gokeepasslib's Clone, because a history version identifies the same entry.
// The copy must be deep: applyDraft replaces e.Values, and a shared backing
// array would let the live edit rewrite the archived version.
func snapshot(e *gokeepasslib.Entry) gokeepasslib.Entry {
	out := *e
	out.Histories = nil
	out.Values = make([]gokeepasslib.ValueData, len(e.Values))
	copy(out.Values, e.Values)
	out.Binaries = make([]gokeepasslib.BinaryReference, len(e.Binaries))
	copy(out.Binaries, e.Binaries)
	out.CustomData = make([]gokeepasslib.CustomData, len(e.CustomData))
	copy(out.CustomData, e.CustomData)
	return out
}

// archive pushes the entry's current state onto its history, then trims to
// the database's configured limit.
func (v *Vault) archive(e *gokeepasslib.Entry) {
	if len(e.Histories) == 0 {
		e.Histories = []gokeepasslib.History{{}}
	}
	e.Histories[0].Entries = append(e.Histories[0].Entries, snapshot(e))
	v.trimHistory(e)
}
