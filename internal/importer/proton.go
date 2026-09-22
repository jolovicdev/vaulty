package importer

import (
	"encoding/json"
	"sort"
	"strings"
)

const sourceProton = "Proton Pass"

// protonTrashed is the item state Proton Pass gives an item in its trash.
const protonTrashed = 2

type protonExport struct {
	Vaults map[string]struct {
		Name  string       `json:"name"`
		Items []protonItem `json:"items"`
	} `json:"vaults"`
}

type protonItem struct {
	State      int    `json:"state"`
	AliasEmail string `json:"aliasEmail"`
	Data       struct {
		Type     string `json:"type"`
		Metadata struct {
			Name string `json:"name"`
			Note string `json:"note"`
		} `json:"metadata"`
		ExtraFields []any           `json:"extraFields"`
		Content     json.RawMessage `json:"content"`
	} `json:"data"`
}

type protonLogin struct {
	ItemEmail    string   `json:"itemEmail"`
	ItemUsername string   `json:"itemUsername"`
	Password     string   `json:"password"`
	URLs         []string `json:"urls"`
	// autofillUrls replaced urls in newer exports; either may be present.
	AutofillURLs []struct {
		URL string `json:"url"`
	} `json:"autofillUrls"`
	TotpURI  string `json:"totpUri"`
	Passkeys []any  `json:"passkeys"`
}

func protonJSON(data []byte) (Result, error) {
	res := Result{Source: sourceProton}
	var exp protonExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return res, err
	}

	// Vault ids are random, so walk them in name order to keep the groups
	// in a predictable order.
	ids := make([]string, 0, len(exp.Vaults))
	for id := range exp.Vaults {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return exp.Vaults[ids[i]].Name < exp.Vaults[ids[j]].Name })

	var lost losses
	for _, id := range ids {
		pv := exp.Vaults[id]
		for _, it := range pv.Items {
			if it.State == protonTrashed {
				lost.trashed++
				continue
			}
			e := newEntry([]string{pv.Name}, it.Data.Metadata.Name)
			if err := protonContent(e, it, &lost); err != nil {
				return res, err
			}
			protonExtra(e, it.Data.ExtraFields)
			e.note(it.Data.Metadata.Note)
			res.Entries = append(res.Entries, e.done())
		}
	}
	res.Warnings = lost.warnings()
	return res, nil
}

// protonContent maps the type-specific part of an item. Logins and aliases
// have a shape of their own; every other type (card, identity, SSH key,
// Wi-Fi, custom) is a record of named values, taken field by field.
func protonContent(e *entry, it protonItem, lost *losses) error {
	switch it.Data.Type {
	case "login":
		var c protonLogin
		if err := json.Unmarshal(it.Data.Content, &c); err != nil {
			return err
		}
		protonUser(e, c.ItemUsername, c.ItemEmail)
		e.password(c.Password)
		for _, u := range c.AutofillURLs {
			e.url(u.URL)
		}
		for _, u := range c.URLs {
			e.url(u)
		}
		e.totp(c.TotpURI)
		lost.passkeys += len(c.Passkeys)
	case "alias":
		e.username(it.AliasEmail)
	default:
		var c map[string]any
		if len(it.Data.Content) > 0 {
			if err := json.Unmarshal(it.Data.Content, &c); err != nil {
				return err
			}
		}
		e.fields(c, func(_ string, v any) { protonExtra(e, v) })
	}
	return nil
}

// protonUser fills the username from the username field, falling back to
// the email one. Proton Pass keeps both, and older exports had only the
// email; when both are set the email is kept as a field.
func protonUser(e *entry, username, email string) {
	e.username(username)
	if e.d.Username == "" {
		e.username(email)
		return
	}
	if strings.TrimSpace(email) != e.d.Username {
		e.field("Email", email, false)
	}
}

// protonExtra reads a list of Proton Pass extra fields, or of sections that
// each hold such a list.
func protonExtra(e *entry, v any) {
	list, _ := v.([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		if fields, ok := m["sectionFields"]; ok {
			protonExtra(e, fields)
			continue
		}
		name := str(m["fieldName"])
		data, _ := m["data"].(map[string]any)
		switch str(m["type"]) {
		case "totp":
			e.totp(str(data["totpUri"]))
		case "hidden":
			e.field(name, str(data["content"]), true)
		case "timestamp":
			e.field(name, str(data["timestamp"]), false)
		default:
			e.field(name, str(data["content"]), false)
		}
	}
}

// protonCSV reads the CSV export. Cards and identities have no columns of
// their own there: Proton Pass writes their content into the note column as
// JSON, with the real note inside it.
func protonCSV(t table) Result {
	res := Result{Source: sourceProton}
	for _, row := range t.rows {
		e := newEntry([]string{t.get(row, "vault")}, t.get(row, "name"))
		protonUser(e, t.get(row, "username"), t.get(row, "email"))
		e.password(t.get(row, "password"))
		for _, u := range strings.Split(t.get(row, "url"), ",") {
			e.url(u)
		}
		e.totp(t.get(row, "totp"))

		note := t.get(row, "note")
		if typ := t.get(row, "type"); typ == "creditCard" || typ == "identity" {
			var c map[string]any
			if json.Unmarshal([]byte(note), &c) == nil {
				note = str(c["note"])
				delete(c, "note")
				e.fields(c, func(_ string, v any) { protonExtra(e, v) })
			}
		}
		e.note(note)
		res.Entries = append(res.Entries, e.done())
	}
	return res
}
