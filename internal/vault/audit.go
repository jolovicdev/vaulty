package vault

import (
	"crypto/sha256"
	"sort"
	"strings"
	"time"

	"github.com/tobischo/gokeepasslib/v3"

	"github.com/jolovicdev/vaulty/internal/strength"
)

const expiringSoonWindow = 30 * 24 * time.Hour

// AuditFinding describes an entry with a weak chosen password. It never
// carries the password itself.
type AuditFinding struct {
	Entry   Meta    `json:"entry"`
	Score   int     `json:"score"`
	Label   string  `json:"label"`
	Warning string  `json:"warning"`
	Entropy float64 `json:"entropy"`
}

// ReusedGroup lists two or more active entries that share the same password.
type ReusedGroup struct {
	Entries []Meta `json:"entries"`
}

// AuditReport summarises password health across all active entries without
// disclosing any secret.
type AuditReport struct {
	Weak         []AuditFinding `json:"weak"`
	Reused       []ReusedGroup  `json:"reused"`
	Expired      []Meta         `json:"expired"`
	ExpiringSoon []Meta         `json:"expiringSoon"`
	TotalChecked int            `json:"totalChecked"`
}

// Audit inspects every active (non-recycled) entry for weak passwords, reused
// passwords, and expired or soon-to-expire entries. Passwords are hashed and
// scored inside Go and never leave this package.
func (v *Vault) Audit(dict strength.Dictionary, now time.Time) (AuditReport, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if err := v.checkOpen(); err != nil {
		return AuditReport{}, err
	}

	report := AuditReport{
		Weak:         []AuditFinding{},
		Reused:       []ReusedGroup{},
		Expired:      []Meta{},
		ExpiringSoon: []Meta{},
	}
	byHash := map[[32]byte][]Meta{}
	soonCutoff := now.Add(expiringSoonWindow)

	v.walk(func(g *gokeepasslib.Group, path string, inBin bool) bool {
		if inBin {
			return true
		}
		for i := range g.Entries {
			e := &g.Entries[i]
			m := v.meta(e, g, path, false)
			report.TotalChecked++

			if e.Times.Expires.Bool && e.Times.ExpiryTime != nil && !e.Times.ExpiryTime.Time.IsZero() {
				exp := e.Times.ExpiryTime.Time
				switch {
				case exp.Before(now):
					m.Expired = true
					report.Expired = append(report.Expired, m)
				case !exp.After(soonCutoff):
					report.ExpiringSoon = append(report.ExpiringSoon, m)
				}
			}

			pw := value(e, FieldPassword)
			if pw == "" {
				continue
			}
			sum := sha256.Sum256([]byte(pw))
			byHash[sum] = append(byHash[sum], m)

			est := strength.Estimate(pw, dict)
			if !est.Machine && est.Score <= 1 {
				report.Weak = append(report.Weak, AuditFinding{
					Entry:   m,
					Score:   est.Score,
					Label:   est.Label,
					Warning: est.Warning,
					Entropy: est.Entropy,
				})
			}
		}
		return true
	})

	for _, group := range byHash {
		if len(group) < 2 {
			continue
		}
		sortMetaByTitle(group)
		report.Reused = append(report.Reused, ReusedGroup{Entries: group})
	}

	sort.Slice(report.Weak, func(i, j int) bool {
		if report.Weak[i].Entropy != report.Weak[j].Entropy {
			return report.Weak[i].Entropy < report.Weak[j].Entropy
		}
		return strings.ToLower(report.Weak[i].Entry.Title) < strings.ToLower(report.Weak[j].Entry.Title)
	})
	sort.Slice(report.Reused, func(i, j int) bool {
		if len(report.Reused[i].Entries) != len(report.Reused[j].Entries) {
			return len(report.Reused[i].Entries) > len(report.Reused[j].Entries)
		}
		return strings.ToLower(report.Reused[i].Entries[0].Title) < strings.ToLower(report.Reused[j].Entries[0].Title)
	})
	sortMetaByExpiry(report.Expired)
	sortMetaByExpiry(report.ExpiringSoon)

	return report, nil
}

func sortMetaByTitle(s []Meta) {
	sort.Slice(s, func(i, j int) bool {
		return strings.ToLower(s[i].Title) < strings.ToLower(s[j].Title)
	})
}

func sortMetaByExpiry(s []Meta) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].ExpiryTime != nil && s[j].ExpiryTime != nil && !s[i].ExpiryTime.Equal(*s[j].ExpiryTime) {
			return s[i].ExpiryTime.Before(*s[j].ExpiryTime)
		}
		return strings.ToLower(s[i].Title) < strings.ToLower(s[j].Title)
	})
}
