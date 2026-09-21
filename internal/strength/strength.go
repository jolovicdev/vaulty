// Package strength estimates how hard a password is to guess.
//
// This is a small pattern-based estimator in the spirit of zxcvbn, not a port
// of it. It finds the cheapest way to build the password out of recognised
// pieces (a dictionary word, a run of repeats, a digit sequence, a keyboard
// run) and charges brute force for whatever is left. The number it reports is
// an upper bound on difficulty for an attacker who knows these patterns, so
// it is deliberately pessimistic and must not be read as a guarantee.
package strength

import (
	"math"
	"strings"
	"unicode"
)

// Result is the estimate for one secret.
type Result struct {
	// Score is 0 (very weak) to 4 (strong), the same banding zxcvbn uses.
	Score int `json:"score"`
	// Entropy is log2 of the estimated guesses needed.
	Entropy float64 `json:"entropy"`
	// Guesses is the estimated number of attempts to find it.
	Guesses float64 `json:"guesses"`
	// Label is a short word for the score.
	Label string `json:"label"`
	// Warning names the weakest pattern found, empty when there is none.
	Warning string `json:"warning"`

	// Machine reports that nothing in the value looks like a human choice:
	// no dictionary word, no sequence, no repeat, no keyboard run, and
	// enough entropy that it cannot have been typed from memory.
	//
	// It matters because a score is advice, and advice only helps for a
	// secret the user picks. Calling an API key "very strong" tells them
	// nothing they can act on: they did not choose it and they cannot
	// change it. For these values the caller should report the length and
	// stop, rather than grading something that is not theirs to grade.
	Machine bool `json:"machine"`

	// Issuer names the service whose key format this matches, when the
	// value carries a recognisable prefix. Empty otherwise.
	Issuer string `json:"issuer"`
}

// Dictionary is the word list matched against the password. The caller
// supplies it so this package carries no data of its own; vaulty passes the
// EFF list the generator embeds.
type Dictionary map[string]int

// NewDictionary indexes words by rank, so a common word costs fewer guesses
// than a rare one.
func NewDictionary(words []string) Dictionary {
	d := make(Dictionary, len(words))
	for i, w := range words {
		lw := strings.ToLower(w)
		if _, seen := d[lw]; !seen {
			d[lw] = i + 1
		}
	}
	return d
}

const minDictWord = 4

// Estimate scores password against dict. A nil dict disables word matching.
func Estimate(password string, dict Dictionary) Result {
	if password == "" {
		return Result{Label: label(0), Warning: "empty"}
	}

	if issuer, ok := tokenIssuer(password); ok {
		// A recognised key format is machine issued by definition. Report
		// its size and who issued it; a band would be noise.
		return Result{
			Score:   4,
			Entropy: round(math.Log2(bruteForceGuesses(password, classSize(password)))),
			Label:   "machine issued",
			Machine: true,
			Issuer:  issuer,
		}
	}

	runes := []rune(password)
	n := len(runes)

	// Brute force is charged against the alphabet of the whole password, not
	// of the piece. Charging per piece would let a mixed password be split
	// into single-class chunks, each cheap, and score far below what an
	// attacker covering the full character set would actually pay.
	alphabet := classSize(password)

	// best[i] is the cheapest guess count for the first i runes, from[i] is
	// where the piece ending at i started, and cause[i] names the pattern
	// that explained it. from is what makes the reconstruction below
	// possible: without it the causes are per position rather than per
	// piece, and patterns the cheapest split never used get reported.
	best := make([]float64, n+1)
	from := make([]int, n+1)
	cause := make([]string, n+1)
	best[0] = 1
	for i := 1; i <= n; i++ {
		best[i] = math.Inf(1)
	}

	for i := 0; i < n; i++ {
		if math.IsInf(best[i], 1) {
			continue
		}
		for j := i + 1; j <= n; j++ {
			piece := string(runes[i:j])
			g, why := pieceGuesses(piece, dict, alphabet)
			// A candidate is only worth taking if it beats what we have.
			if total := best[i] * g; total < best[j] {
				best[j] = total
				from[j] = i
				cause[j] = why
			}
		}
	}

	guesses := best[n]
	if guesses < 1 {
		guesses = 1
	}
	entropy := math.Log2(guesses)
	score := band(entropy)

	// Only the pieces on the cheapest split describe how the value is
	// actually built; everything else was a candidate that lost.
	used, covered := causesOnPath(from, cause, n)

	// A long random string will contain a three character keyboard run or
	// sequence by chance, so the test is how much of the value those
	// patterns explain, not whether any exists at all.
	machine := entropy >= machineEntropy && covered*machineCoverage < n

	out := Result{
		Score:   score,
		Entropy: round(entropy),
		Guesses: guesses,
		Label:   label(score),
		Warning: worstCause(used, score),
		Machine: machine,
	}
	if machine {
		out.Label = "random"
		out.Warning = ""
	}
	return out
}

const (
	// machineEntropy is the point above which a value cannot plausibly have
	// been chosen and remembered by a person: 12 characters drawn from a
	// mixed alphabet, near enough.
	machineEntropy = 72

	// machineCoverage is the reciprocal of the share of the value that
	// recognised patterns may explain before it stops looking machine
	// generated. Three means up to a third.
	machineCoverage = 3
)

// causesOnPath walks the cheapest split back from the end. It returns the
// pattern names the split actually used, in reading order, and how many
// runes those patterns cover.
func causesOnPath(from []int, cause []string, n int) (names []string, covered int) {
	for j := n; j > 0; {
		next := from[j]
		if cause[j] != "" {
			names = append(names, cause[j])
			covered += j - next
		}
		if next >= j {
			break // no predecessor recorded; nothing more to walk
		}
		j = next
	}
	// Reverse, so the names read left to right like the value does.
	for i, k := 0, len(names)-1; i < k; i, k = i+1, k-1 {
		names[i], names[k] = names[k], names[i]
	}
	return names, covered
}

// tokenPrefixes are key formats common enough to be worth naming. The list
// is deliberately short: a prefix that is not here simply falls through to
// the ordinary estimate, so being incomplete costs nothing.
var tokenPrefixes = []struct {
	prefix string
	issuer string
}{
	{"ghp_", "GitHub personal access token"},
	{"gho_", "GitHub OAuth token"},
	{"ghs_", "GitHub server token"},
	{"github_pat_", "GitHub fine-grained token"},
	{"glpat-", "GitLab personal access token"},
	{"sk-", "OpenAI API key"},
	{"sk-ant-", "Anthropic API key"},
	{"xoxb-", "Slack bot token"},
	{"xoxp-", "Slack user token"},
	{"AKIA", "AWS access key id"},
	{"ASIA", "AWS temporary access key id"},
	{"AIza", "Google API key"},
	{"ya29.", "Google OAuth token"},
	{"npm_", "npm access token"},
	{"dop_v1_", "DigitalOcean token"},
	{"shpat_", "Shopify access token"},
	{"pk_live_", "Stripe publishable key"},
	{"sk_live_", "Stripe secret key"},
	{"rk_live_", "Stripe restricted key"},
	{"hf_", "Hugging Face token"},
	{"figd_", "Figma token"},
	{"SG.", "SendGrid API key"},
	{"eyJ", "JSON Web Token"},
}

// minTokenLength keeps a short string that merely starts with "sk-" from
// being reported as a key.
const minTokenLength = 16

// tokenIssuer matches the value against the known key formats, longest
// prefix first so sk-ant- beats sk-.
func tokenIssuer(s string) (string, bool) {
	if len(s) < minTokenLength {
		return "", false
	}
	best, bestLen := "", 0
	for _, t := range tokenPrefixes {
		if len(t.prefix) > bestLen && strings.HasPrefix(s, t.prefix) {
			best, bestLen = t.issuer, len(t.prefix)
		}
	}
	return best, best != ""
}

// pieceGuesses returns the guess count for one substring and the name of the
// pattern that explains it.
func pieceGuesses(s string, dict Dictionary, alphabet int) (float64, string) {
	if g, why, ok := dictionaryGuesses(s, dict); ok {
		return g, why
	}
	if g, ok := repeatGuesses(s); ok {
		return g, "a repeated character"
	}
	if g, ok := sequenceGuesses(s); ok {
		return g, "a character sequence"
	}
	if g, ok := keyboardGuesses(s); ok {
		return g, "a keyboard run"
	}
	return bruteForceGuesses(s, alphabet), ""
}

// dictionaryGuesses matches the piece as a word, optionally capitalised,
// reversed, or written with common letter-for-symbol substitutions. Each
// extra transformation multiplies the cost, because an attacker has to try
// them all.
func dictionaryGuesses(s string, dict Dictionary) (float64, string, bool) {
	if dict == nil || len([]rune(s)) < minDictWord {
		return 0, "", false
	}
	lower := strings.ToLower(s)
	variants := []struct {
		word       string
		multiplier float64
		why        string
	}{
		{lower, 1, "a dictionary word"},
		{reverse(lower), 2, "a reversed dictionary word"},
		{unleet(lower), 4, "a dictionary word with letters swapped for symbols"},
	}
	for _, v := range variants {
		rank, ok := dict[v.word]
		if !ok {
			continue
		}
		g := float64(rank) * v.multiplier
		if s != lower {
			// Capitalisation is cheap to try, so it only doubles the cost.
			g *= 2
		}
		return g, v.why, true
	}
	return 0, "", false
}

var leet = map[rune]rune{'4': 'a', '@': 'a', '8': 'b', '3': 'e', '6': 'g', '1': 'l', '!': 'i', '0': 'o', '5': 's', '7': 't', '$': 's'}

func unleet(s string) string {
	return strings.Map(func(r rune) rune {
		if v, ok := leet[r]; ok {
			return v
		}
		return r
	}, s)
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// repeatGuesses handles "aaaa": the attacker guesses the character then the
// length.
func repeatGuesses(s string) (float64, bool) {
	r := []rune(s)
	if len(r) < 3 {
		return 0, false
	}
	for _, c := range r[1:] {
		if c != r[0] {
			return 0, false
		}
	}
	return float64(classSize(string(r[0])) * len(r)), true
}

// sequenceGuesses handles "1234" and "wxyz" in either direction.
func sequenceGuesses(s string) (float64, bool) {
	r := []rune(s)
	if len(r) < 3 {
		return 0, false
	}
	step := r[1] - r[0]
	if step != 1 && step != -1 {
		return 0, false
	}
	for i := 2; i < len(r); i++ {
		if r[i]-r[i-1] != step {
			return 0, false
		}
	}
	// Start point, direction, length.
	return float64(classSize(s) * 2 * len(r)), true
}

var keyboardRows = []string{
	"`1234567890-=",
	"qwertyuiop[]\\",
	"asdfghjkl;'",
	"zxcvbnm,./",
}

// keyboardGuesses handles adjacent runs along one QWERTY row, like "asdf".
func keyboardGuesses(s string) (float64, bool) {
	r := []rune(strings.ToLower(s))
	if len(r) < 3 {
		return 0, false
	}
	for _, row := range keyboardRows {
		idx := strings.IndexRune(row, r[0])
		if idx < 0 {
			continue
		}
		for _, step := range []int{1, -1} {
			ok := true
			for i := 1; i < len(r); i++ {
				p := idx + step*i
				if p < 0 || p >= len(row) || rune(row[p]) != r[i] {
					ok = false
					break
				}
			}
			if ok {
				return float64(len(keyboardRows) * len(row) * 2 * len(r)), true
			}
		}
	}
	return 0, false
}

func bruteForceGuesses(s string, alphabet int) float64 {
	if alphabet < 2 {
		alphabet = 2
	}
	return math.Pow(float64(alphabet), float64(len([]rune(s))))
}

// classSize is the size of the alphabet an attacker would have to cover to
// produce every character in s.
func classSize(s string) int {
	var lower, upper, digit, symbol, other bool
	for _, r := range s {
		switch {
		case unicode.IsLower(r) && r < 128:
			lower = true
		case unicode.IsUpper(r) && r < 128:
			upper = true
		case unicode.IsDigit(r) && r < 128:
			digit = true
		case r < 128:
			symbol = true
		default:
			other = true
		}
	}
	size := 0
	if lower {
		size += 26
	}
	if upper {
		size += 26
	}
	if digit {
		size += 10
	}
	if symbol {
		size += 33
	}
	if other {
		// One plane of printable non-ASCII, charged conservatively.
		size += 100
	}
	return size
}

// Bit thresholds for the five bands. 28 and 36 bits mark passwords an online
// and an offline attacker respectively can exhaust; 60 and 80 mark what
// resists a funded offline attack with current hardware.
func band(entropy float64) int {
	switch {
	case entropy < 28:
		return 0
	case entropy < 36:
		return 1
	case entropy < 60:
		return 2
	case entropy < 80:
		return 3
	default:
		return 4
	}
}

func label(score int) string {
	switch score {
	case 0:
		return "very weak"
	case 1:
		return "weak"
	case 2:
		return "fair"
	case 3:
		return "strong"
	default:
		return "very strong"
	}
}

// worstCause names a pattern only when it is worth acting on. The first one
// on the path is the one to mention: it is where the value starts being
// guessable.
func worstCause(used []string, score int) string {
	if score >= 3 || len(used) == 0 {
		return ""
	}
	return "contains " + used[0]
}

func round(f float64) float64 {
	return math.Round(f*10) / 10
}
