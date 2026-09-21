package vault

import (
	"sort"
	"strings"
	"unicode"

	"github.com/tobischo/gokeepasslib/v3"
)

// Search scores every entry against a query and returns the matches best
// first. It looks at title, username, url and tags only, so a search never
// touches a password or a note.
func (v *Vault) Search(query string, opts ListOptions) ([]Meta, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if err := v.checkOpen(); err != nil {
		return nil, err
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return v.listLocked(opts)
	}

	needle := fold(q)
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
			m := v.meta(e, g, path, inBin)
			score := scoreEntry(needle, m)
			if score <= 0 {
				continue
			}
			m.Score = score
			out = append(out, m)
		}
		return true
	})

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out, nil
}

// Field weights. Title dominates, then username, then url, then tags, so
// typing "git" ranks an entry titled GitHub above one merely tagged git.
const (
	weightTitle    = 8
	weightUsername = 4
	weightURL      = 3
	weightTag      = 3
)

func scoreEntry(needle string, m Meta) int {
	total := 0
	total += weightTitle * matchScore(needle, fold(m.Title))
	total += weightUsername * matchScore(needle, fold(m.Username))
	total += weightURL * matchScore(needle, fold(m.URL))
	for _, t := range m.Tags {
		if s := matchScore(needle, fold(t)); s > 0 {
			total += weightTag * s
			break
		}
	}
	return total
}

// matchScore rates how well needle matches haystack, both already folded.
// A prefix beats a substring, a substring beats a scattered subsequence, and
// anything else scores zero. The start of a word sits between the first two,
// and is tested before the substring because it always is one as well.
func matchScore(needle, haystack string) int {
	if needle == "" || haystack == "" {
		return 0
	}
	switch {
	case haystack == needle:
		return 10
	case strings.HasPrefix(haystack, needle):
		return 8
	case wordPrefix(needle, haystack):
		return 6
	case strings.Contains(haystack, needle):
		return 5
	case subsequence(needle, haystack):
		return 2
	}
	return 0
}

// wordPrefix reports whether needle starts any word of haystack, so "hub"
// matches "git hub" and "git-hub" but not "github".
func wordPrefix(needle, haystack string) bool {
	for _, word := range strings.FieldsFunc(haystack, isSeparator) {
		if strings.HasPrefix(word, needle) {
			return true
		}
	}
	return false
}

func isSeparator(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("-_./:@+", r)
}

// subsequence reports whether needle's runes appear in haystack in order.
func subsequence(needle, haystack string) bool {
	n := []rune(needle)
	i := 0
	for _, r := range haystack {
		if i < len(n) && r == n[i] {
			i++
		}
	}
	return i == len(n)
}

func fold(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
