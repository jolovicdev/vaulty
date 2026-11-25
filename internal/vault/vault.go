package vault

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tobischo/gokeepasslib/v3"
	w "github.com/tobischo/gokeepasslib/v3/wrappers"
)

// kdfArgon2id is the KDBX 4 KDF UUID for Argon2id. gokeepasslib derives only
// Argon2d and AES-KDF, so matching this UUID lets Open report the real reason
// instead of a misleading wrong-password error.
var kdfArgon2id = []byte{
	0x9e, 0x29, 0x8b, 0x19, 0x56, 0xdb, 0x47, 0x73,
	0xb2, 0x3d, 0xfc, 0x3e, 0xc6, 0xf0, 0xa1, 0xe6,
}

// Credentials is the unlock material for a database. Either field may be
// empty, but not both.
type Credentials struct {
	Password string
	KeyFile  string
}

func (c Credentials) build() (*gokeepasslib.DBCredentials, error) {
	switch {
	case c.Password != "" && c.KeyFile != "":
		return gokeepasslib.NewPasswordAndKeyCredentials(c.Password, c.KeyFile)
	case c.KeyFile != "":
		return gokeepasslib.NewKeyCredentials(c.KeyFile)
	case c.Password != "":
		return gokeepasslib.NewPasswordCredentials(c.Password), nil
	default:
		return nil, fmt.Errorf("vault: no password and no key file given")
	}
}

// Vault is an open KDBX database. Every method is safe for concurrent use.
type Vault struct {
	mu   sync.RWMutex
	path string
	db   *gokeepasslib.Database

	// fingerprint of the file as it was when we read it, used to refuse a
	// save that would clobber another writer (Syncthing, KeePassXC).
	loaded fileStamp

	dirty bool

	// backedUp records that this session has already copied the file it
	// found on disk into the rolling backups. Every edit saves, so a backup
	// per save would push all of them out within a few edits; one per
	// session keeps them a session apart.
	backedUp bool
}

type fileStamp struct {
	ModTime time.Time
	Size    int64
}

func stat(path string) (fileStamp, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, err
	}
	return fileStamp{ModTime: fi.ModTime(), Size: fi.Size()}, nil
}

// Open reads and decrypts the database at path.
func Open(path string, creds Credentials) (*Vault, error) {
	dbc, err := creds.build()
	if err != nil {
		return nil, err
	}
	return openWith(path, dbc)
}

// openWith is Open once the credentials are built. Reload and MergeFromDisk
// use it to reread a file with the key material this session already holds,
// so resolving a sync conflict does not make the user retype the master
// password.
func openWith(path string, dbc *gokeepasslib.DBCredentials) (*Vault, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	stamp, err := stat(path)
	if err != nil {
		return nil, err
	}

	db := gokeepasslib.NewDatabase()
	db.Credentials = dbc
	if err := gokeepasslib.NewDecoder(f).Decode(db); err != nil {
		if db.Header != nil && db.Header.IsKdbx4() &&
			db.Header.FileHeaders != nil &&
			db.Header.FileHeaders.KdfParameters != nil &&
			bytes.Equal(db.Header.FileHeaders.KdfParameters.UUID, kdfArgon2id) {
			return nil, ErrArgon2id
		}
		// Decode returns a combined "bad credentials or corrupt file" error.
		// It never contains the password, so it is safe to surface.
		return nil, err
	}
	if err := db.UnlockProtectedEntries(); err != nil {
		return nil, err
	}

	return &Vault{path: path, db: db, loaded: stamp}, nil
}

// KDF cost for a vault created here. It is set explicitly because the header
// gokeepasslib builds asks for 1 MiB and two passes, which costs whoever holds
// the file a couple of milliseconds per guess. 64 MiB is what KeePass and
// KeePassXC ask for. The pass count is a compromise: the derivation runs on
// every unlock and again on every save, and every edit saves.
const (
	kdfMemory     = 64 << 20 // bytes
	kdfIterations = 10
)

// Create writes a new, empty KDBX 4.0 database (Argon2d + ChaCha20, which is
// what NewKDBX40Header sets, at the cost above) and returns it open.
func Create(path string, creds Credentials) (*Vault, error) {
	dbc, err := creds.build()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("vault: %s already exists", filepath.Base(path))
	}

	db := gokeepasslib.NewDatabase(gokeepasslib.WithDatabaseKDBXVersion40())
	db.Header.FileHeaders.KdfParameters.Memory = kdfMemory
	db.Header.FileHeaders.KdfParameters.Iterations = kdfIterations
	db.Credentials = dbc
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	db.Content.Meta.DatabaseName = name

	root := gokeepasslib.NewGroup()
	root.Name = name
	db.Content.Root.Groups = []gokeepasslib.Group{root}

	db.Content.Meta.RecycleBinEnabled = w.NewBoolWrapper(true)

	v := &Vault{path: path, db: db, dirty: true}
	if err := v.Save(); err != nil {
		return nil, err
	}
	return v, nil
}

// Path is the file this vault was read from.
func (v *Vault) Path() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.path
}

// Dirty reports whether there are changes not yet written to disk.
func (v *Vault) Dirty() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.dirty
}

// FormatVersion is "3.1", "4.0" or "4.1" for the file as loaded.
func (v *Vault) FormatVersion() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.db == nil {
		return ""
	}
	h := v.db.Header
	if !h.IsKdbx4() {
		return "3.1"
	}
	return fmt.Sprintf("%d.%d", h.Signature.MajorVersion, h.Signature.MinorVersion)
}

// Close drops the database and its key material. The values are unreachable
// afterwards; when the collector runs is not ours to decide.
func (v *Vault) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.db != nil {
		v.db.Credentials = nil
		v.db.Content = nil
		v.db.Header = nil
		v.db = nil
	}
	v.dirty = false
}

func (v *Vault) checkOpen() error {
	if v.db == nil || v.db.Content == nil || v.db.Content.Root == nil {
		return ErrLocked
	}
	return nil
}

// recycleBinUUID returns the recycle bin group id, or the zero UUID when the
// database has none.
func (v *Vault) recycleBinUUID() gokeepasslib.UUID {
	if v.db.Content.Meta == nil {
		return gokeepasslib.UUID{}
	}
	if !v.db.Content.Meta.RecycleBinEnabled.Bool {
		return gokeepasslib.UUID{}
	}
	return v.db.Content.Meta.RecycleBinUUID
}

// walk visits every group depth first, passing the group, its path and
// whether it sits at or under the recycle bin.
func (v *Vault) walk(fn func(g *gokeepasslib.Group, path string, inBin bool) bool) {
	bin := v.recycleBinUUID()
	var rec func(groups []gokeepasslib.Group, prefix string, inBin bool) bool
	rec = func(groups []gokeepasslib.Group, prefix string, inBin bool) bool {
		for i := range groups {
			g := &groups[i]
			path := g.Name
			if prefix != "" {
				path = prefix + "/" + g.Name
			}
			under := inBin || (!bin.IsZero() && g.UUID.Compare(bin))
			if !fn(g, path, under) {
				return false
			}
			if !rec(g.Groups, path, under) {
				return false
			}
		}
		return true
	}
	rec(v.db.Content.Root.Groups, "", false)
}

// findEntry locates an entry by id and returns it along with its owning group.
func (v *Vault) findEntry(id string) (*gokeepasslib.Entry, *gokeepasslib.Group, string, bool, error) {
	if err := v.checkOpen(); err != nil {
		return nil, nil, "", false, err
	}
	var (
		entry *gokeepasslib.Entry
		owner *gokeepasslib.Group
		path  string
		inBin bool
	)
	v.walk(func(g *gokeepasslib.Group, p string, bin bool) bool {
		for i := range g.Entries {
			if entryID(&g.Entries[i]) == id {
				entry, owner, path, inBin = &g.Entries[i], g, p, bin
				return false
			}
		}
		return true
	})
	if entry == nil {
		return nil, nil, "", false, ErrNotFound
	}
	return entry, owner, path, inBin, nil
}

func (v *Vault) findGroup(id string) (*gokeepasslib.Group, error) {
	if err := v.checkOpen(); err != nil {
		return nil, err
	}
	var found *gokeepasslib.Group
	v.walk(func(g *gokeepasslib.Group, _ string, _ bool) bool {
		if groupID(g) == id {
			found = g
			return false
		}
		return true
	})
	if found == nil {
		return nil, ErrGroupNotFound
	}
	return found, nil
}

// Groups returns the group tree with per-group entry counts.
func (v *Vault) Groups() ([]GroupNode, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if err := v.checkOpen(); err != nil {
		return nil, err
	}
	bin := v.recycleBinUUID()

	var conv func(groups []gokeepasslib.Group, prefix string) []GroupNode
	conv = func(groups []gokeepasslib.Group, prefix string) []GroupNode {
		out := make([]GroupNode, 0, len(groups))
		for i := range groups {
			g := &groups[i]
			path := g.Name
			if prefix != "" {
				path = prefix + "/" + g.Name
			}
			out = append(out, GroupNode{
				ID:        groupID(g),
				Name:      g.Name,
				Path:      path,
				Count:     len(g.Entries),
				IsRecycle: !bin.IsZero() && g.UUID.Compare(bin),
				Children:  conv(g.Groups, path),
			})
		}
		return out
	}
	return conv(v.db.Content.Root.Groups, ""), nil
}

// Tags returns every distinct tag in the database, sorted.
func (v *Vault) Tags() ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if err := v.checkOpen(); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	v.walk(func(g *gokeepasslib.Group, _ string, inBin bool) bool {
		if inBin {
			return true
		}
		for i := range g.Entries {
			for _, t := range splitTags(g.Entries[i].Tags) {
				seen[t] = struct{}{}
			}
		}
		return true
	})
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sortStrings(out)
	return out, nil
}

// DatabaseName is the name stored in the KDBX metadata.
func (v *Vault) DatabaseName() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.checkOpen() != nil || v.db.Content.Meta == nil {
		return ""
	}
	return v.db.Content.Meta.DatabaseName
}

type credSnapshot struct {
	creds     *gokeepasslib.DBCredentials
	seed      []byte
	iv        []byte
	salt      [32]byte
	hasSalt   bool
	transform []byte
	dirty     bool
}

func takeCredSnapshot(v *Vault) credSnapshot {
	fh := v.db.Header.FileHeaders
	s := credSnapshot{
		creds:     v.db.Credentials,
		seed:      slices.Clone(fh.MasterSeed),
		iv:        slices.Clone(fh.EncryptionIV),
		transform: slices.Clone(fh.TransformSeed),
		dirty:     v.dirty,
	}
	if fh.KdfParameters != nil {
		s.salt = fh.KdfParameters.Salt
		s.hasSalt = true
	}
	return s
}

func (s credSnapshot) restore(v *Vault) {
	fh := v.db.Header.FileHeaders
	v.db.Credentials = s.creds
	fh.MasterSeed = s.seed
	fh.EncryptionIV = s.iv
	if fh.KdfParameters != nil && s.hasSalt {
		fh.KdfParameters.Salt = s.salt
	}
	fh.TransformSeed = s.transform
	v.dirty = s.dirty
}

// ChangeCredentials replaces the vault's master password and/or key file,
// rotates its KDF salt, master seed and encryption IV, and writes the file
// immediately. If saving fails (for instance because the file changed on disk),
// the previous credentials and header material are restored so Reload and
// MergeFromDisk can still open the file on disk.
func (v *Vault) ChangeCredentials(next Credentials) error {
	dbc, err := next.build()
	if err != nil {
		return err
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.checkOpen(); err != nil {
		return err
	}

	prev := takeCredSnapshot(v)
	fh := v.db.Header.FileHeaders

	if err := refreshBytes(&fh.MasterSeed, len(prev.seed)); err != nil {
		prev.restore(v)
		return err
	}
	if err := refreshBytes(&fh.EncryptionIV, len(prev.iv)); err != nil {
		prev.restore(v)
		return err
	}
	if fh.KdfParameters != nil && prev.hasSalt {
		if _, err := rand.Read(fh.KdfParameters.Salt[:]); err != nil {
			prev.restore(v)
			return err
		}
	}
	if len(prev.transform) > 0 {
		if err := refreshBytes(&fh.TransformSeed, len(prev.transform)); err != nil {
			prev.restore(v)
			return err
		}
	}

	v.db.Credentials = dbc
	v.dirty = true
	if err := v.saveLocked(); err != nil {
		prev.restore(v)
		return err
	}
	return nil
}

func refreshBytes(dst *[]byte, n int) error {
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	*dst = buf
	return nil
}
