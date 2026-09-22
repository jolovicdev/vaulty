package importer

import (
	"encoding/json"
	"strings"
)

const sourceBitwarden = "Bitwarden"

// bitwardenHidden is the custom field type Bitwarden masks. A linked field
// (type 3) has a null value, so the empty-value rule leaves it out.
const bitwardenHidden = 1

type bitwardenExport struct {
	Folders     []bitwardenGroup `json:"folders"`
	Collections []bitwardenGroup `json:"collections"`
	Items       []bitwardenItem  `json:"items"`
}

type bitwardenGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type bitwardenItem struct {
	Name          string   `json:"name"`
	Notes         string   `json:"notes"`
	FolderID      string   `json:"folderId"`
	CollectionIDs []string `json:"collectionIds"`
	DeletedDate   string   `json:"deletedDate"`
	Fields        []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Type  int    `json:"type"`
	} `json:"fields"`
	Login *struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Totp     string `json:"totp"`
		URIs     []struct {
			URI string `json:"uri"`
		} `json:"uris"`
		Fido2Credentials []any `json:"fido2Credentials"`
	} `json:"login"`
	// The other item types are records of named values. Bank accounts,
	// licences and passports are newer types with the same shape.
	Card           map[string]any `json:"card"`
	Identity       map[string]any `json:"identity"`
	SSHKey         map[string]any `json:"sshKey"`
	BankAccount    map[string]any `json:"bankAccount"`
	DriversLicense map[string]any `json:"driversLicense"`
	Passport       map[string]any `json:"passport"`
}

func bitwardenJSON(data []byte) (Result, error) {
	res := Result{Source: sourceBitwarden}
	var exp bitwardenExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return res, err
	}

	// A personal export files items in folders; an organisation export in
	// collections. Both use "/" in a name for nesting.
	names := map[string]string{}
	for _, g := range append(exp.Folders, exp.Collections...) {
		names[g.ID] = g.Name
	}

	var lost losses
	for _, it := range exp.Items {
		if it.DeletedDate != "" {
			lost.trashed++
			continue
		}
		group := it.FolderID
		if group == "" && len(it.CollectionIDs) > 0 {
			group = it.CollectionIDs[0]
		}
		e := newEntry(splitPath(names[group]), it.Name)
		if l := it.Login; l != nil {
			e.username(l.Username)
			e.password(l.Password)
			for _, u := range l.URIs {
				e.url(u.URI)
			}
			e.totp(l.Totp)
			lost.passkeys += len(l.Fido2Credentials)
		}
		for _, rec := range []map[string]any{it.Card, it.Identity, it.SSHKey, it.BankAccount, it.DriversLicense, it.Passport} {
			e.fields(rec, nil)
		}
		for _, f := range it.Fields {
			e.field(f.Name, f.Value, f.Type == bitwardenHidden)
		}
		e.note(it.Notes)
		res.Entries = append(res.Entries, e.done())
	}
	res.Warnings = lost.warnings()
	return res, nil
}

// bitwardenCSV reads the CSV export, which holds logins and notes only.
// login_uri is a comma separated list, and fields is one "name: value" per
// line.
func bitwardenCSV(t table) Result {
	res := Result{Source: sourceBitwarden}
	for _, row := range t.rows {
		folder := t.get(row, "folder")
		if folder == "" {
			folder, _, _ = strings.Cut(t.get(row, "collections"), ",")
		}
		e := newEntry(splitPath(folder), t.get(row, "name"))
		e.username(t.get(row, "login_username"))
		e.password(t.get(row, "login_password"))
		for _, u := range strings.Split(t.get(row, "login_uri"), ",") {
			e.url(u)
		}
		e.totp(t.get(row, "login_totp"))
		for _, line := range strings.Split(t.get(row, "fields"), "\n") {
			line = strings.TrimSuffix(line, "\r")
			if i := strings.LastIndex(line, ": "); i >= 0 {
				e.field(line[:i], line[i+2:], false)
			}
		}
		e.note(t.get(row, "notes"))
		res.Entries = append(res.Entries, e.done())
	}
	return res
}
