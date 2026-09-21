// Package vault wraps a KDBX database file with the operations the
// application needs. It has no UI dependencies and is exercised entirely
// from go test.
//
// Memory hygiene note: Go's garbage collector relocates and copies values,
// so this package cannot guarantee that a secret is erased from memory once
// it has been read. What it does instead is bounded: secrets are returned
// one field at a time, they are never placed in the metadata structures the
// UI holds, and Close drops the whole database object so the collector can
// reclaim it. KDBX in-memory "protection" is a stream cipher whose key sits
// in the same address space; it obscures a heap dump, it is not a security
// boundary.
package vault

import (
	"errors"
	"time"
)

// Field names as KeePass stores them in an entry's string values.
const (
	FieldTitle    = "Title"
	FieldUserName = "UserName"
	FieldPassword = "Password"
	FieldURL      = "URL"
	FieldNotes    = "Notes"
	FieldTOTPSeed = "otp"
)

var (
	ErrLocked        = errors.New("vault is locked")
	ErrNotFound      = errors.New("entry not found")
	ErrGroupNotFound = errors.New("group not found")
	ErrNoRecycleBin  = errors.New("recycle bin is disabled for this database")
	ErrDiskChanged   = errors.New("vault file changed on disk since it was opened")
	ErrReadOnlyField = errors.New("field cannot be written directly")
)

// Meta is the entry projection the UI list is allowed to hold. It carries no
// secret: no password, no notes, no protected custom field.
type Meta struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Username     string    `json:"username"`
	URL          string    `json:"url"`
	Tags         []string  `json:"tags"`
	GroupID      string    `json:"groupId"`
	GroupPath    string    `json:"groupPath"`
	Modified     time.Time `json:"modified"`
	Created      time.Time `json:"created"`
	HasTOTP      bool      `json:"hasTotp"`
	HasNotes     bool      `json:"hasNotes"`
	Expired      bool      `json:"expired"`
	InRecycleBin bool      `json:"inRecycleBin"`
	Score        int       `json:"score"`
}

// CustomField describes a custom string value without disclosing it.
type CustomField struct {
	Key       string `json:"key"`
	Protected bool   `json:"protected"`
	// Value is populated only for unprotected fields. A protected field is
	// read through Reveal.
	Value string `json:"value"`
}

// Detail is everything the detail pane may hold before the user asks to
// reveal a field. Password and notes are absent by construction.
type Detail struct {
	Meta
	Custom       []CustomField `json:"custom"`
	HistoryCount int           `json:"historyCount"`
	PasswordSet  bool          `json:"passwordSet"`
}

// Draft is the input for creating or updating an entry. It does carry
// secrets, which is why it only ever travels from the UI into this package
// and is never returned. An update whose TOTPSeed is empty keeps the seed the
// entry already has.
type Draft struct {
	Title    string            `json:"title"`
	Username string            `json:"username"`
	Password string            `json:"password"`
	URL      string            `json:"url"`
	Notes    string            `json:"notes"`
	Tags     []string          `json:"tags"`
	TOTPSeed string            `json:"totpSeed"`
	Custom   map[string]string `json:"custom"`
	// ProtectedCustom lists which Custom keys must be stored protected.
	ProtectedCustom []string `json:"protectedCustom"`
	GroupID         string   `json:"groupId"`
	// RemoveTOTP takes the one-time code off the entry on an update. An empty
	// TOTPSeed cannot ask for that, because it means unchanged.
	RemoveTOTP bool `json:"removeTotp"`
}

// GroupNode is one node of the group tree.
type GroupNode struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Path      string      `json:"path"`
	Count     int         `json:"count"`
	IsRecycle bool        `json:"isRecycleBin"`
	Children  []GroupNode `json:"children"`
}

// HistoryVersion is one past revision of an entry, again without secrets.
type HistoryVersion struct {
	Index    int       `json:"index"`
	Title    string    `json:"title"`
	Username string    `json:"username"`
	URL      string    `json:"url"`
	Modified time.Time `json:"modified"`
}
