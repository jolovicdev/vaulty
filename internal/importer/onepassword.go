package importer

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const sourceOnePassword = "1Password"

// onePasswordArchived tags an item 1Password had archived. Archiving there
// hides an item without deleting it, so it is imported, and the tag keeps
// it findable.
const onePasswordArchived = "archived"

type onePasswordExport struct {
	Accounts []struct {
		Attrs struct {
			AccountName string `json:"accountName"`
			Name        string `json:"name"`
		} `json:"attrs"`
		Vaults []struct {
			Attrs struct {
				Name string `json:"name"`
			} `json:"attrs"`
			Items []onePasswordItem `json:"items"`
		} `json:"vaults"`
	} `json:"accounts"`
}

type onePasswordItem struct {
	State   string `json:"state"`
	Details struct {
		LoginFields []struct {
			Value       string `json:"value"`
			Name        string `json:"name"`
			FieldType   string `json:"fieldType"`
			Designation string `json:"designation"`
		} `json:"loginFields"`
		NotesPlain string `json:"notesPlain"`
		// Password is where a Password item, which has no login fields,
		// keeps its one value.
		Password string `json:"password"`
		Sections []struct {
			Title  string `json:"title"`
			Fields []struct {
				Title string `json:"title"`
				ID    string `json:"id"`
				// Value has exactly one key, which names the kind of value.
				Value map[string]json.RawMessage `json:"value"`
			} `json:"fields"`
		} `json:"sections"`
	} `json:"details"`
	Overview struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		URLs  []struct {
			URL string `json:"url"`
		} `json:"urls"`
		Tags []string `json:"tags"`
	} `json:"overview"`
}

// onePasswordData reads export.data, the JSON inside a .1pux archive.
func onePasswordData(data []byte) (Result, error) {
	res := Result{Source: sourceOnePassword}
	var exp onePasswordExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return res, err
	}
	for _, acct := range exp.Accounts {
		for _, pv := range acct.Vaults {
			folder := []string{pv.Attrs.Name}
			// A family or team export can hold several accounts, and their
			// vaults may share names.
			if len(exp.Accounts) > 1 {
				name := acct.Attrs.AccountName
				if name == "" {
					name = acct.Attrs.Name
				}
				folder = []string{name, pv.Attrs.Name}
			}
			for _, it := range pv.Items {
				res.Entries = append(res.Entries, onePasswordEntry(folder, it).done())
			}
		}
	}
	return res, nil
}

func onePasswordEntry(folder []string, it onePasswordItem) *entry {
	e := newEntry(folder, it.Overview.Title)
	e.url(it.Overview.URL)
	for _, u := range it.Overview.URLs {
		e.url(u.URL)
	}
	for _, t := range it.Overview.Tags {
		e.tag(t)
	}
	if it.State == onePasswordArchived {
		e.tag(onePasswordArchived)
	}

	for _, f := range it.Details.LoginFields {
		switch f.Designation {
		case "username":
			e.username(f.Value)
		case "password":
			e.password(f.Value)
		default:
			e.field(f.Name, f.Value, f.FieldType == "P")
		}
	}
	e.password(it.Details.Password)

	for _, s := range it.Details.Sections {
		for _, f := range s.Fields {
			name := f.Title
			if name == "" {
				name = s.Title
			}
			for kind, raw := range f.Value {
				onePasswordValue(e, name, f.ID, kind, raw)
			}
		}
	}
	e.note(it.Details.NotesPlain)
	return e
}

// onePasswordValue files one section field by the kind of value it holds.
// A login-like item that keeps its username, password or address in a
// section rather than in login fields, such as a server or a database, has
// them moved into the standard fields. A value that does not decode as its
// kind says is left out rather than failing the whole import.
func onePasswordValue(e *entry, name, id, kind string, raw json.RawMessage) {
	switch kind {
	case "totp":
		var s string
		if json.Unmarshal(raw, &s) == nil {
			e.totp(s)
		}
	case "concealed", "creditCardNumber":
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return
		}
		if id == "password" && e.d.Password == "" {
			e.password(s)
			return
		}
		e.field(name, s, true)
	case "email":
		var v struct {
			Address string `json:"email_address"`
		}
		if json.Unmarshal(raw, &v) == nil {
			e.field(name, v.Address, false)
		}
	case "date":
		var n int64
		if json.Unmarshal(raw, &n) == nil && n != 0 {
			e.field(name, time.Unix(n, 0).UTC().Format("2006-01-02"), false)
		}
	case "monthYear":
		// 202512 is December 2025.
		var n int
		if json.Unmarshal(raw, &n) == nil && n != 0 {
			e.field(name, fmt.Sprintf("%02d/%04d", n%100, n/100), false)
		}
	case "address":
		var a struct {
			Street  string `json:"street"`
			City    string `json:"city"`
			State   string `json:"state"`
			Zip     string `json:"zip"`
			Country string `json:"country"`
		}
		if json.Unmarshal(raw, &a) != nil {
			return
		}
		var parts []string
		for _, p := range []string{a.Street, a.City, a.State, a.Zip, a.Country} {
			if p = strings.TrimSpace(p); p != "" {
				parts = append(parts, p)
			}
		}
		e.field(name, strings.Join(parts, ", "), false)
	case "sshKey":
		var k struct {
			Metadata struct {
				PrivateKey  string `json:"privateKey"`
				PublicKey   string `json:"publicKey"`
				Fingerprint string `json:"fingerprint"`
			} `json:"metadata"`
		}
		if json.Unmarshal(raw, &k) != nil {
			return
		}
		e.field("Private key", k.Metadata.PrivateKey, true)
		e.field("Public key", k.Metadata.PublicKey, false)
		e.field("Fingerprint", k.Metadata.Fingerprint, false)
	case "reference":
		// A link to another item by its id, which means nothing here.
	default:
		// string, url, phone, menu, gender and the rest are plain text.
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return
		}
		switch {
		case id == "username" && e.d.Username == "":
			e.username(s)
		case (kind == "url" || id == "url") && e.d.URL == "":
			e.url(s)
		default:
			// A passport or licence number is stored as plain text there.
			e.field(name, s, sensitive[strings.ToLower(id)])
		}
	}
}

// onePasswordCSV reads the CSV export of 1Password 8, which holds logins and
// passwords only.
func onePasswordCSV(t table) Result {
	res := Result{Source: sourceOnePassword}
	for _, row := range t.rows {
		e := newEntry(nil, t.get(row, "title"))
		e.username(t.get(row, "username"))
		e.password(t.get(row, "password"))
		e.url(t.get(row, "url"))
		e.url(t.get(row, "website"))
		for _, col := range []string{"otpauth", "one-time password", "totp"} {
			e.totp(t.get(row, col))
		}
		for _, tag := range strings.FieldsFunc(t.get(row, "tags"), func(r rune) bool { return r == ',' || r == ';' }) {
			e.tag(tag)
		}
		if strings.EqualFold(strings.TrimSpace(t.get(row, "archived")), "true") {
			e.tag(onePasswordArchived)
		}
		e.note(t.get(row, "notes"))
		res.Entries = append(res.Entries, e.done())
	}
	return res
}
