package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jolovicdev/vaulty/internal/clipboard"
	"github.com/jolovicdev/vaulty/internal/generator"
	"github.com/jolovicdev/vaulty/internal/importer"
	"github.com/jolovicdev/vaulty/internal/settings"
	"github.com/jolovicdev/vaulty/internal/strength"
	"github.com/jolovicdev/vaulty/internal/totp"
	"github.com/jolovicdev/vaulty/internal/vault"
)

// eventLocked tells the frontend to drop every cached value and show the
// unlock screen. It is emitted for an idle lock as well as an explicit one.
const eventLocked = "vault:locked"

// eventCloseRequested asks the frontend to put the unsaved changes question,
// because the window was closed while the vault had unsaved work.
const eventCloseRequested = "vault:close-requested"

// eventConflict says an autosave found the file changed underneath us. The
// edit is still held in memory and the file is untouched.
const eventConflict = "vault:conflict"

// App is the Wails binding layer. It maps calls onto the internal packages
// and owns the session state; the logic lives in those packages, not here.
type App struct {
	ctx context.Context

	mu    sync.Mutex
	v     *vault.Vault
	idle  *time.Timer
	clips *clipboard.Manager

	// quitting records that the user has answered the unsaved changes
	// question, so the close that follows is not questioned again.
	quitting bool

	store *settings.Store
	// cached holds the preferences so that Touch and the copy bindings do
	// not read the config file on every call. SaveSettings replaces it.
	cached settings.Settings
	// dict is the wordlist the strength meter matches against, built once
	// because indexing 7776 words on every keystroke would be wasteful.
	dict strength.Dictionary
}

// NewApp wires the session. A failure to locate the config directory is not
// fatal: the app runs with defaults and says so when the settings screen
// asks for the path.
func NewApp() *App {
	a := &App{clips: clipboard.New(), cached: settings.Default()}
	if store, err := settings.NewStore(); err == nil {
		a.store = store
		a.cached = store.Load()
	}
	if words, err := generator.Words(); err == nil {
		a.dict = strength.NewDictionary(words)
	}
	return a
}

// prefs returns the cached preferences.
func (a *App) prefs() settings.Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cached
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown drops the vault and wipes the clipboard on the way out, so
// closing the window is as safe as locking.
func (a *App) shutdown(context.Context) {
	a.Lock()
}

// beforeClose stops the window closing while there is unsaved work and asks
// the frontend to put the question.
//
// The question is deliberately not an OS dialog. On Windows, Wails maps a
// question dialog onto MessageBox with MB_YESNO: the Buttons field is
// ignored, only Yes and No are shown, and the result comes back as "Yes" or
// "No" rather than as any label given here. A three way choice cannot be
// expressed that way, and matching on custom labels there matches nothing at
// all. Asking in the app's own dialog behaves the same on both platforms and
// is under this code's control.
func (a *App) beforeClose(context.Context) bool {
	if !a.hasUnsavedWork() {
		return false
	}
	if a.ctx == nil {
		// No frontend to ask, so nothing could answer and release the
		// block. Letting the close through is the only outcome that cannot
		// strand the user in a window they are unable to shut.
		return false
	}
	runtime.EventsEmit(a.ctx, eventCloseRequested)
	return true
}

// hasUnsavedWork is the decision behind beforeClose, kept apart from the
// event it sends so the rule can be tested without a running frontend.
func (a *App) hasUnsavedWork() bool {
	a.mu.Lock()
	v, quitting := a.v, a.quitting
	a.mu.Unlock()

	// Quit was already confirmed, so this close must not be questioned
	// again or the app could never shut.
	if quitting {
		return false
	}
	return v != nil && v.Dirty()
}

// ConfirmQuit ends the session after the user has answered the unsaved
// changes question. A save that fails leaves the app open and returns the
// error, because quitting would discard the very work the user asked to
// keep.
func (a *App) ConfirmQuit(save bool) error {
	if save {
		a.mu.Lock()
		v := a.v
		a.mu.Unlock()
		if v != nil {
			if err := v.Save(); err != nil {
				return err
			}
		}
	}

	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()

	a.Lock()
	runtime.Quit(a.ctx)
	return nil
}

// errNoVault is what every call that needs an open vault returns when there
// is none. It is deliberately free of detail; there is nothing safe to add.
var errNoVault = errors.New("no vault is open")

// withVault runs fn with the open vault and treats the call as user
// activity, restarting the idle countdown.
func (a *App) withVault(fn func(v *vault.Vault) error) error {
	if err := a.withVaultQuiet(fn); err != nil {
		return err
	}
	a.Touch()
	return nil
}

// withVaultQuiet runs fn without touching the idle timer. It is for calls the
// frontend makes on a timer rather than in response to the user: a selected
// entry refreshes its one-time code every period, and if that counted as
// activity the vault would never lock.
func (a *App) withVaultQuiet(fn func(v *vault.Vault) error) error {
	a.mu.Lock()
	v := a.v
	a.mu.Unlock()
	if v == nil {
		return errNoVault
	}
	return fn(v)
}

// Settings state

// LoadSettings returns the stored preferences, dropping recent vaults whose
// file has since gone.
func (a *App) LoadSettings() settings.Settings {
	if a.store == nil {
		return a.prefs()
	}
	s := a.store.Load()
	s.PruneMissing()

	a.mu.Lock()
	a.cached = s
	a.mu.Unlock()
	return s
}

// SaveSettings persists the preferences and re-arms the idle timer with the
// new timeout.
func (a *App) SaveSettings(s settings.Settings) error {
	if a.store == nil {
		return errors.New("settings directory is unavailable on this system")
	}
	if err := a.store.Save(s); err != nil {
		return err
	}
	a.mu.Lock()
	a.cached = s
	a.mu.Unlock()
	a.Touch()
	return nil
}

// SettingsPath is shown in the settings screen so the user can find the file.
func (a *App) SettingsPath() string {
	if a.store == nil {
		return ""
	}
	return a.store.Path()
}

// Session state

// Status is what the frontend needs to decide which screen to show.
type Status struct {
	Unlocked       bool   `json:"unlocked"`
	Path           string `json:"path"`
	Name           string `json:"name"`
	FormatVersion  string `json:"formatVersion"`
	Dirty          bool   `json:"dirty"`
	ClipboardWorks bool   `json:"clipboardWorks"`
}

// Status reports the session without touching the idle timer, so polling it
// cannot keep the vault unlocked.
func (a *App) Status() Status {
	a.mu.Lock()
	v := a.v
	a.mu.Unlock()

	out := Status{ClipboardWorks: clipboard.Available()}
	if v == nil {
		return out
	}
	out.Unlocked = true
	out.Path = v.Path()
	out.Name = v.DatabaseName()
	out.FormatVersion = v.FormatVersion()
	out.Dirty = v.Dirty()
	return out
}

// Unlock opens a vault and starts the session.
func (a *App) Unlock(path, password, keyFile string) (Status, error) {
	v, err := vault.Open(path, vault.Credentials{Password: password, KeyFile: keyFile})
	if err != nil {
		// vault.Open's error names the file or says the credentials and the
		// file cannot be told apart. Neither contains the password.
		return Status{}, err
	}

	a.mu.Lock()
	old := a.v
	a.v = v
	a.mu.Unlock()
	if old != nil {
		old.Close()
	}

	a.remember(path, keyFile)
	a.Touch()
	return a.Status(), nil
}

// CreateVault makes a new KDBX 4 file and opens it.
func (a *App) CreateVault(path, password, keyFile string) (Status, error) {
	if filepath.Ext(path) == "" {
		path += ".kdbx"
	}
	v, err := vault.Create(path, vault.Credentials{Password: password, KeyFile: keyFile})
	if err != nil {
		return Status{}, err
	}

	a.mu.Lock()
	old := a.v
	a.v = v
	a.mu.Unlock()
	if old != nil {
		old.Close()
	}

	a.remember(path, keyFile)
	a.Touch()
	return a.Status(), nil
}

func (a *App) remember(path, keyFile string) {
	if a.store == nil {
		return
	}
	s := a.store.Load()
	if err := s.Remember(path, keyFile, time.Now()); err != nil {
		return
	}
	if err := a.store.Save(s); err != nil {
		return
	}
	a.mu.Lock()
	a.cached = s
	a.mu.Unlock()
}

// ForgetRecent drops a vault from the recent list.
func (a *App) ForgetRecent(path string) error {
	if a.store == nil {
		return nil
	}
	s := a.store.Load()
	s.Forget(path)
	return a.store.Save(s)
}

// Lock ends the session: the database object and its key material go, the
// clipboard is wiped if it still holds our value, and the frontend is told
// to reset. Go's collector decides when the memory is actually reused, which
// is why nothing here claims to have erased it.
func (a *App) Lock() {
	a.mu.Lock()
	v := a.v
	a.v = nil
	if a.idle != nil {
		a.idle.Stop()
		a.idle = nil
	}
	a.mu.Unlock()

	if v != nil {
		v.Close()
	}
	a.clips.ClearNow()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, eventLocked)
	}
}

// Touch restarts the idle countdown. The frontend calls it on real input,
// and every binding that reaches the vault calls it too.
func (a *App) Touch() {
	d := a.prefs().IdleLock()

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.v == nil {
		return
	}
	if a.idle != nil {
		a.idle.Stop()
	}
	a.idle = time.AfterFunc(d, a.Lock)
}

// Reading entries

// List returns entry metadata for the current filter. No secret is included.
func (a *App) List(opts vault.ListOptions) ([]vault.Meta, error) {
	var out []vault.Meta
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.List(opts)
		return err
	})
	return out, err
}

// Search runs the fuzzy match in Go over title, username, url and tags.
func (a *App) Search(query string, opts vault.ListOptions) ([]vault.Meta, error) {
	var out []vault.Meta
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.Search(query, opts)
		return err
	})
	return out, err
}

// Groups returns the group tree.
func (a *App) Groups() ([]vault.GroupNode, error) {
	var out []vault.GroupNode
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.Groups()
		return err
	})
	return out, err
}

// Tags returns every tag in the vault.
func (a *App) Tags() ([]string, error) {
	var out []string
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.Tags()
		return err
	})
	return out, err
}

// Detail returns one entry without its secrets.
func (a *App) Detail(id string) (vault.Detail, error) {
	var out vault.Detail
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.Detail(id)
		return err
	})
	return out, err
}

// Reveal returns one field of one entry. This is the only binding that
// returns a secret, and the frontend drops the value when the field is
// hidden, when the selection changes, and on lock.
func (a *App) Reveal(id, field string) (string, error) {
	var out string
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.Reveal(id, field)
		return err
	})
	return out, err
}

// History lists an entry's past revisions.
func (a *App) History(id string) ([]vault.HistoryVersion, error) {
	var out []vault.HistoryVersion
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.History(id)
		return err
	})
	return out, err
}

// RevealHistory returns one field of one past revision.
func (a *App) RevealHistory(id string, index int, field string) (string, error) {
	var out string
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.RevealHistory(id, index, field)
		return err
	})
	return out, err
}

// Writing entries

// AddEntry creates an entry and returns its id.
func (a *App) AddEntry(d vault.Draft) (string, error) {
	var id string
	if err := a.withVault(func(v *vault.Vault) error {
		var err error
		id, err = v.Add(d)
		return err
	}); err != nil {
		return "", err
	}
	return id, a.autosave()
}

// UpdateEntry replaces an entry's fields, keeping the previous state in the
// entry's history.
func (a *App) UpdateEntry(id string, d vault.Draft) error {
	return a.saving(func(v *vault.Vault) error { return v.Update(id, d) })
}

// DeleteEntry recycles an entry, or removes it permanently when it is
// already in the bin.
func (a *App) DeleteEntry(id string) error {
	return a.saving(func(v *vault.Vault) error { return v.Delete(id) })
}

// RestoreEntry moves an entry out of the recycle bin.
func (a *App) RestoreEntry(id, groupID string) error {
	return a.saving(func(v *vault.Vault) error { return v.Restore(id, groupID) })
}

// EmptyRecycleBin permanently removes everything in the bin.
func (a *App) EmptyRecycleBin() error {
	return a.saving(func(v *vault.Vault) error { return v.EmptyRecycleBin() })
}

// MoveEntry relocates an entry to another group.
func (a *App) MoveEntry(id, groupID string) error {
	return a.saving(func(v *vault.Vault) error { return v.Move(id, groupID) })
}

// AddGroup creates a subgroup and returns its id.
func (a *App) AddGroup(parentID, name string) (string, error) {
	var id string
	if err := a.withVault(func(v *vault.Vault) error {
		var err error
		id, err = v.AddGroup(parentID, name)
		return err
	}); err != nil {
		return "", err
	}
	return id, a.autosave()
}

// RestoreVersion copies a past revision back onto the live entry.
func (a *App) RestoreVersion(id string, index int) error {
	return a.saving(func(v *vault.Vault) error { return v.RestoreVersion(id, index) })
}

// Saving

// Save writes the vault back to disk.
func (a *App) Save() error {
	return a.withVault(func(v *vault.Vault) error { return v.Save() })
}

// autosave writes after a change, and is called by every binding that makes
// one. There is no Save button: saving is atomic and refuses to overwrite a
// file another writer touched, so there is nothing a manual save protects
// against that this does not.
//
// A conflict is the one case it cannot resolve on its own. The change stays
// in memory, the file is untouched, and the frontend is told to ask; that is
// the same dialog a manual save produced.
func (a *App) autosave() error {
	a.mu.Lock()
	v := a.v
	a.mu.Unlock()
	if v == nil {
		return nil
	}

	err := v.Save()
	if errors.Is(err, vault.ErrDiskChanged) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, eventConflict)
		}
		// Not returned as an error: the edit itself succeeded and is still
		// held, so the caller has nothing to undo.
		return nil
	}
	return err
}

// saving wraps a write so the change and its save are one call to the
// frontend. A failed write is returned; a failed save is too, except for the
// conflict case above.
func (a *App) saving(change func(v *vault.Vault) error) error {
	if err := a.withVault(change); err != nil {
		return err
	}
	return a.autosave()
}

// DiskState reports whether another writer has touched the file.
func (a *App) DiskState() (vault.DiskState, error) {
	var out vault.DiskState
	err := a.withVault(func(v *vault.Vault) error {
		var err error
		out, err = v.CheckDisk()
		return err
	})
	return out, err
}

// ConflictChoice names how the user wants a changed-on-disk conflict
// resolved.
type ConflictChoice string

const (
	// ConflictReload throws away this session's edits.
	ConflictReload ConflictChoice = "reload"
	// ConflictSaveCopy keeps both files.
	ConflictSaveCopy ConflictChoice = "copy"
	// ConflictMerge folds the two together by entry UUID and modification
	// time.
	ConflictMerge ConflictChoice = "merge"
)

// ConflictResult tells the UI what happened, so it can report it in one line.
type ConflictResult struct {
	Choice ConflictChoice    `json:"choice"`
	Path   string            `json:"path"`
	Merge  vault.MergeResult `json:"merge"`
}

// ResolveConflict applies the user's choice and, for the two choices that
// end in a write, performs it.
func (a *App) ResolveConflict(choice ConflictChoice) (ConflictResult, error) {
	out := ConflictResult{Choice: choice}
	err := a.withVault(func(v *vault.Vault) error {
		switch choice {
		case ConflictReload:
			if err := v.Reload(); err != nil {
				return err
			}
			out.Path = v.Path()
			return nil
		case ConflictSaveCopy:
			out.Path = v.ConflictCopyPath(time.Now())
			return v.SaveAs(out.Path)
		case ConflictMerge:
			res, err := v.MergeFromDisk()
			if err != nil {
				return err
			}
			out.Merge = res
			out.Path = v.Path()
			return v.Save()
		}
		return fmt.Errorf("unknown conflict choice %q", choice)
	})
	return out, err
}

// Backups lists the rolling backups beside the vault.
func (a *App) Backups() ([]string, error) {
	var out []string
	err := a.withVault(func(v *vault.Vault) error {
		out = v.Backups()
		return nil
	})
	return out, err
}

var errEmptyExport = errors.New("the export holds no entries to import")

// ImportPreview says what an export holds before anything is written. It
// carries a count and names, never an entry: the export is read on this
// side both times, so no imported secret passes through the webview.
type ImportPreview struct {
	Source   string   `json:"source"`
	Entries  int      `json:"entries"`
	Group    string   `json:"group"`
	Warnings []string `json:"warnings"`
}

// PickImportFile asks for another password manager's export.
func (a *App) PickImportFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import from another password manager",
		Filters: []runtime.FileFilter{
			{DisplayName: "Proton Pass, Bitwarden and 1Password exports", Pattern: "*.zip;*.json;*.csv;*.1pux"},
			{DisplayName: "All files", Pattern: "*"},
		},
	})
}

// PreviewImport reads an export and reports what importing it would add.
func (a *App) PreviewImport(path string) (ImportPreview, error) {
	var out ImportPreview
	err := a.withVault(func(*vault.Vault) error {
		res, err := readExport(path)
		if err != nil {
			return err
		}
		out = ImportPreview{
			Source:   res.Source,
			Entries:  len(res.Entries),
			Group:    importGroup(res.Source),
			Warnings: append([]string{}, res.Warnings...),
		}
		return nil
	})
	return out, err
}

// ImportFile adds every entry of an export to a new group, in one write and
// one save. A save that fails takes the group back out, so the dialog can
// offer the import again without doubling it. A save refused because the
// file changed on disk is the conflict case every write shares: the entries
// stay in memory and the conflict dialog takes over.
func (a *App) ImportFile(path string) error {
	return a.withVault(func(v *vault.Vault) error {
		res, err := readExport(path)
		if err != nil {
			return err
		}
		dirty := v.Dirty()
		id, err := v.Import(importGroup(res.Source), res.Entries)
		if err != nil {
			return err
		}
		if err := a.autosave(); err != nil {
			return errors.Join(err, v.UndoImport(id, dirty))
		}
		return nil
	})
}

func readExport(path string) (importer.Result, error) {
	res, err := importer.Read(path)
	if err != nil {
		return res, err
	}
	if len(res.Entries) == 0 {
		return res, errEmptyExport
	}
	return res, nil
}

func importGroup(source string) string {
	return "Imported from " + source
}

// Clipboard

// CopyResult is what a copy returns: never the value, only when it will be
// wiped.
type CopyResult struct {
	Field     string `json:"field"`
	ExpiresAt int64  `json:"expiresAt"`
	Seconds   int    `json:"seconds"`
}

// CopyField puts one field of one entry on the clipboard. The value is read
// and written inside Go; it is never sent to the frontend for a copy.
func (a *App) CopyField(id, field string) (CopyResult, error) {
	d := a.prefs().ClipboardClear()
	var out CopyResult
	err := a.withVault(func(v *vault.Vault) error {
		value, err := v.Reveal(id, field)
		if err != nil {
			return err
		}
		if err := a.clips.Copy(value, d); err != nil {
			return err
		}
		out = CopyResult{Field: field, Seconds: int(d.Seconds())}
		if e := a.clips.Expires(); !e.IsZero() {
			out.ExpiresAt = e.UnixMilli()
		}
		return nil
	})
	return out, err
}

// CopyTOTP puts the current one-time code on the clipboard.
func (a *App) CopyTOTP(id string) (CopyResult, error) {
	d := a.prefs().ClipboardClear()
	var out CopyResult
	err := a.withVault(func(v *vault.Vault) error {
		seed, err := v.TOTPSeed(id)
		if err != nil {
			return err
		}
		cfg, err := totp.Parse(seed)
		if err != nil {
			return err
		}
		code, err := cfg.Code(time.Now())
		if err != nil {
			return err
		}
		if err := a.clips.Copy(code, d); err != nil {
			return err
		}
		out = CopyResult{Field: "totp", Seconds: int(d.Seconds())}
		if e := a.clips.Expires(); !e.IsZero() {
			out.ExpiresAt = e.UnixMilli()
		}
		return nil
	})
	return out, err
}

// CopyGenerated puts a value the generator just produced on the clipboard.
//
// Stored secrets stay out of the frontend: a password in the vault is
// copied by id so it never travels to JavaScript. A generated password is
// different, because it is on screen for the user to read before they accept
// it, so it is already there. Routing the copy back through Go anyway is
// what keeps the timed clear and the Windows history exclusion, which
// navigator.clipboard would bypass entirely.
func (a *App) CopyGenerated(value string) (CopyResult, error) {
	if value == "" {
		return CopyResult{}, errors.New("nothing to copy")
	}
	d := a.prefs().ClipboardClear()
	if err := a.clips.Copy(value, d); err != nil {
		return CopyResult{}, err
	}
	out := CopyResult{Field: "generated", Seconds: int(d.Seconds())}
	if e := a.clips.Expires(); !e.IsZero() {
		out.ExpiresAt = e.UnixMilli()
	}
	return out, nil
}

// ClearClipboard wipes the clipboard now, if it still holds our value.
func (a *App) ClearClipboard() {
	a.clips.ClearNow()
}

// TOTP

// TOTPCode is a code and the life left in it, for the countdown ring.
type TOTPCode struct {
	Code      string `json:"code"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	Remaining int    `json:"remaining"`
	Issuer    string `json:"issuer"`
	Account   string `json:"account"`
}

// TOTP returns the current code for an entry.
func (a *App) TOTP(id string) (TOTPCode, error) {
	var out TOTPCode
	err := a.withVaultQuiet(func(v *vault.Vault) error {
		seed, err := v.TOTPSeed(id)
		if err != nil {
			return err
		}
		cfg, err := totp.Parse(seed)
		if err != nil {
			return err
		}
		now := time.Now()
		code, err := cfg.Code(now)
		if err != nil {
			return err
		}
		out = TOTPCode{
			Code:      code,
			Digits:    cfg.Digits,
			Period:    cfg.Period,
			Remaining: int(cfg.Remaining(now).Seconds()),
			Issuer:    cfg.Issuer,
			Account:   cfg.Account,
		}
		return nil
	})
	return out, err
}

// Generator and strength

// GeneratePassword returns a random password. Randomness comes from
// crypto/rand only.
func (a *App) GeneratePassword(o generator.PasswordOptions) (string, error) {
	return generator.Password(o)
}

// GeneratePassphrase returns random words from the embedded EFF long list.
func (a *App) GeneratePassphrase(o generator.PassphraseOptions) (string, error) {
	return generator.Passphrase(o)
}

// DefaultGeneratorOptions gives the UI its starting state.
func (a *App) DefaultGeneratorOptions() struct {
	Password   generator.PasswordOptions   `json:"password"`
	Passphrase generator.PassphraseOptions `json:"passphrase"`
} {
	return struct {
		Password   generator.PasswordOptions   `json:"password"`
		Passphrase generator.PassphraseOptions `json:"passphrase"`
	}{generator.DefaultPasswordOptions(), generator.DefaultPassphraseOptions()}
}

// EstimateStrength scores a candidate password. The value arrives from the
// editor, where the user typed it, and is not stored anywhere by this call.
func (a *App) EstimateStrength(password string) strength.Result {
	return strength.Estimate(password, a.dict)
}

// File pickers
//
// These are the OS dialogs, so the unlock screen does not make the user type
// a path. A cancelled dialog returns an empty string rather than an error,
// which is what the frontend wants: nothing to report, nothing to change.

var vaultFilters = []runtime.FileFilter{
	{DisplayName: "KeePass databases (*.kdbx)", Pattern: "*.kdbx"},
	{DisplayName: "All files", Pattern: "*"},
}

// PickVaultFile asks for an existing vault to open.
func (a *App) PickVaultFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Open a vault",
		Filters:          vaultFilters,
		DefaultDirectory: a.lastVaultDir(),
	})
}

// PickNewVaultPath asks where to create a vault. The dialog handles the
// "this file exists, overwrite?" prompt itself; vault.Create then refuses to
// touch an existing file, so a mistaken confirmation cannot destroy a vault.
func (a *App) PickNewVaultPath() (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:                "Create a vault",
		DefaultFilename:      "vault.kdbx",
		Filters:              vaultFilters,
		DefaultDirectory:     a.lastVaultDir(),
		CanCreateDirectories: true,
	})
}

// PickKeyFile asks for the optional key file that goes with a master
// password. KeePassXC writes .keyx; older files have no extension at all.
func (a *App) PickKeyFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose a key file",
		Filters: []runtime.FileFilter{
			{DisplayName: "Key files (*.keyx, *.key)", Pattern: "*.keyx;*.key"},
			{DisplayName: "All files", Pattern: "*"},
		},
		DefaultDirectory: a.lastVaultDir(),
		ShowHiddenFiles:  true,
	})
}

// lastVaultDir opens the dialog where the user last kept a vault, rather
// than wherever the process happens to have been started.
func (a *App) lastVaultDir() string {
	recent := a.prefs().Recent
	if len(recent) == 0 {
		return ""
	}
	return filepath.Dir(recent[0].Path)
}

// Confirm asks the user a yes or no question in an OS dialog and reports
// whether they agreed.
//
// The answer is matched against the standard button names rather than
// against a label supplied here: Windows shows MessageBox with Yes and No
// and returns those words, ignoring any custom buttons. Anything else,
// including an error, counts as no, so a misread answer can only ever
// cancel a destructive action.
func (a *App) Confirm(title, message string) bool {
	choice, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         title,
		Message:       message,
		DefaultButton: "No",
	})
	if err != nil {
		return false
	}
	switch choice {
	case "Yes", "Ok":
		return true
	default:
		return false
	}
}

// OpenURL hands a URL to the desktop's browser. Nothing in this app fetches
// a URL itself.
func (a *App) OpenURL(id string) error {
	return a.withVault(func(v *vault.Vault) error {
		url, err := v.Reveal(id, vault.FieldURL)
		if err != nil {
			return err
		}
		if url == "" {
			return errors.New("entry has no URL")
		}
		runtime.BrowserOpenURL(a.ctx, browserURL(url))
		return nil
	})
}

// browserURL gives an address stored without a scheme, such as "github.com",
// the https one, which is the rule KeePassXC applies. Wails refuses a URL
// that has no scheme and only logs the refusal, so the action would
// otherwise do nothing at all.
func browserURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "://") {
		return raw
	}
	return "https://" + raw
}
