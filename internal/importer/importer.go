// Package importer reads the unencrypted exports of other password managers
// and turns them into vault drafts. It reads Proton Pass (.zip, .json, .csv),
// and tells the formats apart by content rather than by file name.
//
// Nothing here writes to a vault or keeps a value past the call: Read returns
// the drafts and the caller hands them to vault.Import.
package importer

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/jolovicdev/vaulty/internal/totp"
	"github.com/jolovicdev/vaulty/internal/vault"
)

var (
	ErrUnknownFormat = errors.New("the file is not a Proton Pass export")
	ErrEncrypted     = errors.New("the export is encrypted; export again without a password or encryption")
	ErrTooLarge      = errors.New("the export is larger than an import allows")
)

// maxExport bounds how much a single export, or one file inside an archive,
// may decompress to. A vault of tens of thousands of entries fits well
// inside it; a zip bomb does not.
const maxExport = 256 << 20

// Result is what one export holds.
type Result struct {
	Source  string
	Entries []vault.Imported
	// Warnings names what the export held that a KDBX entry here cannot, one
	// line per kind, so the user knows what to carry over by hand.
	Warnings []string
}

// Read parses the export at path.
func Read(path string) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := readLimited(f)
	if err != nil {
		return Result{}, err
	}
	return parse(data)
}

func parse(data []byte) (Result, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return parseZip(data)
	case bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")):
		return parseJSON(data, 0)
	default:
		return parseCSV(data)
	}
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxExport+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxExport {
		return nil, ErrTooLarge
	}
	return data, nil
}

// parseZip handles the archive export. A Proton Pass .zip holds
// "Proton Pass/data.json", or data.pgp when the user chose to encrypt it.
// Attachments sit beside it under files/, and are counted so the user hears
// that they stayed behind.
func parseZip(data []byte) (Result, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Result{}, err
	}
	var main *zip.File
	files := 0
	for _, f := range zr.File {
		name := f.Name
		switch {
		case strings.HasSuffix(name, "/"):
		case strings.HasPrefix(name, "Proton Pass/files/"):
			files++
		case name == "Proton Pass/data.json":
			main = f
		case name == "Proton Pass/data.pgp":
			return Result{}, ErrEncrypted
		}
	}
	if main == nil {
		return Result{}, ErrUnknownFormat
	}
	rc, err := main.Open()
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = rc.Close() }()
	body, err := readLimited(rc)
	if err != nil {
		return Result{}, err
	}
	return parseJSON(body, files)
}

func parseJSON(data []byte, files int) (Result, error) {
	var probe struct {
		Encrypted bool            `json:"encrypted"`
		Vaults    json.RawMessage `json:"vaults"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Result{}, ErrUnknownFormat
	}
	if probe.Encrypted {
		return Result{}, ErrEncrypted
	}
	var (
		res Result
		err error
	)
	switch {
	case probe.Vaults != nil:
		res, err = protonJSON(data)
	default:
		return Result{}, ErrUnknownFormat
	}
	if err != nil {
		// The decoder's message names a field and a type, never a value.
		return Result{}, fmt.Errorf("reading the %s export: %w", res.Source, err)
	}
	if files > 0 {
		res.Warnings = append(res.Warnings, plural(files, "attached file was", "attached files were")+" not imported")
	}
	return res, nil
}

// parseCSV recognises each app's export by its header row. Columns are
// looked up by name because every app has reordered or added columns
// between versions.
func parseCSV(data []byte) (Result, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 {
		return Result{}, ErrUnknownFormat
	}
	t := newTable(rows)
	if t.has("type", "name", "url", "password", "note", "totp") {
		return protonCSV(t), nil
	}
	return Result{}, ErrUnknownFormat
}

// table is a CSV with its header folded to lower case, so a column is found
// whatever capitalisation the exporting app used.
type table struct {
	cols map[string]int
	rows [][]string
}

func newTable(rows [][]string) table {
	t := table{cols: map[string]int{}, rows: rows[1:]}
	for i, h := range rows[0] {
		t.cols[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return t
}

func (t table) has(names ...string) bool {
	for _, n := range names {
		if _, ok := t.cols[n]; !ok {
			return false
		}
	}
	return true
}

// get returns the named column of row, or the empty string when the export
// has no such column or the row is short.
func (t table) get(row []string, name string) string {
	i, ok := t.cols[name]
	if !ok || i >= len(row) {
		return ""
	}
	return row[i]
}

// losses counts what an export held that cannot be imported. Each kind turns
// into one warning line rather than one per item.
type losses struct {
	trashed  int
	passkeys int
}

func (l losses) warnings() []string {
	var out []string
	if l.trashed > 0 {
		out = append(out, plural(l.trashed, "item in the trash was", "items in the trash were")+" left out")
	}
	if l.passkeys > 0 {
		out = append(out, plural(l.passkeys, "passkey was", "passkeys were")+" not imported")
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// entry builds one draft. The helpers skip empty values, so a caller can
// hand over every field an item has without checking each one.
type entry struct {
	folder []string
	d      vault.Draft
	totps  int
	urls   []string
}

func newEntry(folder []string, title string) *entry {
	var path []string
	for _, f := range folder {
		if f = strings.TrimSpace(f); f != "" {
			path = append(path, f)
		}
	}
	return &entry{folder: path, d: vault.Draft{Title: strings.TrimSpace(title), Custom: map[string]string{}}}
}

func (e *entry) done() vault.Imported {
	return vault.Imported{Folder: e.folder, Draft: e.d}
}

func (e *entry) username(s string) {
	if s = strings.TrimSpace(s); s != "" && e.d.Username == "" {
		e.d.Username = s
	}
}

func (e *entry) password(s string) {
	if s != "" && e.d.Password == "" {
		e.d.Password = s
	}
}

// url sets the entry's URL, and files every further one as KP2A_URL_n,
// which KeePassXC reads as an additional URL of the same entry. Exports list
// some URLs twice, so one the entry already has is skipped.
func (e *entry) url(s string) {
	s = strings.TrimSpace(s)
	if s == "" || slices.Contains(e.urls, s) {
		return
	}
	e.urls = append(e.urls, s)
	if len(e.urls) == 1 {
		e.d.URL = s
		return
	}
	e.field("KP2A_URL_"+strconv.Itoa(len(e.urls)-1), s, false)
}

func (e *entry) note(s string) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
	case e.d.Notes == "":
		e.d.Notes = s
	default:
		e.d.Notes += "\n\n" + s
	}
}

// totp stores a one-time password setting. The first one becomes the
// entry's code. The otp field KeePassXC reads must hold an otpauth URI, so
// a bare base32 secret is wrapped in one. A second code, or a value that is
// neither, is kept as a protected field rather than dropped.
func (e *entry) totp(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if e.d.TOTPSeed == "" {
		if uri, ok := otpURI(s, e.d.Title); ok {
			e.d.TOTPSeed = uri
			return
		}
	}
	e.totps++
	name := "One-time password"
	if e.totps > 1 {
		name += " " + strconv.Itoa(e.totps)
	}
	e.field(name, s, true)
}

func otpURI(s, title string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(s), "otpauth://") {
		secret := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(s))
		s = "otpauth://totp/" + url.PathEscape(title) + "?secret=" + url.QueryEscape(secret)
	}
	if _, err := totp.Parse(s); err != nil {
		return "", false
	}
	return s, true
}

// field adds a custom field. A name that is empty, taken, or one of the
// standard KeePass fields gets a number, since applyDraft would otherwise
// drop or overwrite it.
func (e *entry) field(name, value string, protect bool) {
	if strings.TrimSpace(value) == "" {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Field"
	}
	key := name
	for n := 2; e.taken(key); n++ {
		key = name + " " + strconv.Itoa(n)
	}
	e.d.Custom[key] = value
	if protect {
		e.d.ProtectedCustom = append(e.d.ProtectedCustom, key)
	}
}

func (e *entry) taken(key string) bool {
	_, ok := e.d.Custom[key]
	return ok || vault.IsStandardField(key)
}

// sensitive names, in lower case, the structured values that are secrets:
// card numbers and codes, identity document numbers, key material. They are
// stored protected, like a password. The keys are Proton Pass record keys.
var sensitive = map[string]bool{
	"password": true, "pin": true,
	"number": true, "verificationnumber": true,
	"socialsecuritynumber": true,
	"passportnumber":       true, "licensenumber": true,
	"privatekey": true,
}

// fields adds every string in a structured record, such as a card or an
// identity, as a custom field named after its key. Nested lists of named
// fields are handed to extra.
func (e *entry) fields(content map[string]any, extra func(key string, v any)) {
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch v := content[k].(type) {
		case string:
			e.field(label(k), v, sensitive[strings.ToLower(k)])
		case []any, map[string]any:
			if extra != nil {
				extra(k, v)
			}
		}
	}
}

// str renders a decoded JSON scalar as text. Anything else, including null,
// is empty, which every entry helper treats as absent.
func str(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// label turns a camelCase key into words: "socialSecurityNumber" becomes
// "Social security number".
func label(key string) string {
	var b strings.Builder
	for i, r := range key {
		switch {
		case i == 0:
			b.WriteRune(unicode.ToUpper(r))
		case unicode.IsUpper(r):
			b.WriteRune(' ')
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
