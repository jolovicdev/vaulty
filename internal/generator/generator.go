// Package generator produces random passwords and passphrases. Every random
// value comes from crypto/rand; there is no math/rand anywhere in this
// package and no seeding to get wrong.
package generator

import (
	"crypto/rand"
	"embed"
	"errors"
	"math/big"
	"strconv"
	"strings"
)

//go:embed wordlist.txt
var wordlistFS embed.FS

var (
	// ErrNoCharacters is returned when every character class is disabled.
	ErrNoCharacters = errors.New("generator: no character classes selected")
	// ErrLength is returned for a length outside the supported range.
	ErrLength = errors.New("generator: length out of range")
)

// Character class alphabets. Ambiguous characters are the ones a person
// cannot tell apart in most fonts; excluding them is optional because it
// costs entropy.
const (
	Lower     = "abcdefghijklmnopqrstuvwxyz"
	Upper     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	Digits    = "0123456789"
	Symbols   = "!@#$%^&*()-_=+[]{};:,.?/"
	ambiguous = "0O1lI|5S2Z8B"
)

// Bounds on generated output.
const (
	MinLength = 4
	MaxLength = 256
	MinWords  = 3
	MaxWords  = 24
)

// PasswordOptions controls Password.
type PasswordOptions struct {
	Length           int  `json:"length"`
	Lower            bool `json:"lower"`
	Upper            bool `json:"upper"`
	Digits           bool `json:"digits"`
	Symbols          bool `json:"symbols"`
	ExcludeAmbiguous bool `json:"excludeAmbiguous"`
}

// DefaultPasswordOptions is what the UI starts from.
func DefaultPasswordOptions() PasswordOptions {
	return PasswordOptions{Length: 20, Lower: true, Upper: true, Digits: true, Symbols: true}
}

func (o PasswordOptions) alphabet() string {
	var b strings.Builder
	if o.Lower {
		b.WriteString(Lower)
	}
	if o.Upper {
		b.WriteString(Upper)
	}
	if o.Digits {
		b.WriteString(Digits)
	}
	if o.Symbols {
		b.WriteString(Symbols)
	}
	if !o.ExcludeAmbiguous {
		return b.String()
	}
	var out strings.Builder
	for _, r := range b.String() {
		if !strings.ContainsRune(ambiguous, r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// Password returns a password drawn uniformly from the selected alphabet.
//
// Each selected class is guaranteed to appear at least once, which is what
// most password policies demand. That guarantee costs a little entropy
// against a uniform draw; Entropy reports the honest lower bound for the
// alphabet size and length rather than pretending otherwise.
func Password(o PasswordOptions) (string, error) {
	if o.Length < MinLength || o.Length > MaxLength {
		return "", ErrLength
	}
	alphabet := o.alphabet()
	if alphabet == "" {
		return "", ErrNoCharacters
	}

	classes := o.requiredClasses()
	if len(classes) > o.Length {
		return "", ErrLength
	}

	out := make([]rune, 0, o.Length)
	for _, class := range classes {
		r, err := pickRune(class)
		if err != nil {
			return "", err
		}
		out = append(out, r)
	}
	for len(out) < o.Length {
		r, err := pickRune(alphabet)
		if err != nil {
			return "", err
		}
		out = append(out, r)
	}
	if err := shuffle(out); err != nil {
		return "", err
	}
	return string(out), nil
}

// requiredClasses returns one alphabet per selected class, already filtered
// for ambiguity, skipping any class the filter emptied.
func (o PasswordOptions) requiredClasses() []string {
	var out []string
	for _, pair := range []struct {
		on    bool
		chars string
	}{
		{o.Lower, Lower},
		{o.Upper, Upper},
		{o.Digits, Digits},
		{o.Symbols, Symbols},
	} {
		if !pair.on {
			continue
		}
		chars := pair.chars
		if o.ExcludeAmbiguous {
			chars = strings.Map(func(r rune) rune {
				if strings.ContainsRune(ambiguous, r) {
					return -1
				}
				return r
			}, chars)
		}
		if chars != "" {
			out = append(out, chars)
		}
	}
	return out
}

// PassphraseOptions controls Passphrase.
type PassphraseOptions struct {
	Words      int    `json:"words"`
	Separator  string `json:"separator"`
	Capitalize bool   `json:"capitalize"`
	AddNumber  bool   `json:"addNumber"`
}

// DefaultPassphraseOptions is what the UI starts from.
func DefaultPassphraseOptions() PassphraseOptions {
	return PassphraseOptions{Words: 6, Separator: "-"}
}

// Passphrase returns words drawn from the EFF long wordlist (7776 entries,
// so 12.925 bits per word).
func Passphrase(o PassphraseOptions) (string, error) {
	if o.Words < MinWords || o.Words > MaxWords {
		return "", ErrLength
	}
	list, err := Words()
	if err != nil {
		return "", err
	}

	parts := make([]string, 0, o.Words)
	for i := 0; i < o.Words; i++ {
		n, err := randInt(len(list))
		if err != nil {
			return "", err
		}
		word := list[n]
		if o.Capitalize {
			word = strings.ToUpper(word[:1]) + word[1:]
		}
		parts = append(parts, word)
	}
	phrase := strings.Join(parts, o.Separator)
	if o.AddNumber {
		n, err := randInt(10)
		if err != nil {
			return "", err
		}
		phrase += o.Separator + strconv.Itoa(n)
	}
	return phrase, nil
}

var words []string

// Words returns the embedded EFF long wordlist.
func Words() ([]string, error) {
	if words != nil {
		return words, nil
	}
	data, err := wordlistFS.ReadFile("wordlist.txt")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	words = out
	return words, nil
}

func pickRune(alphabet string) (rune, error) {
	runes := []rune(alphabet)
	n, err := randInt(len(runes))
	if err != nil {
		return 0, err
	}
	return runes[n], nil
}

// randInt returns a uniform value in [0,n) using rejection-free arithmetic
// from crypto/rand via math/big, which is the standard library's own
// unbiased path.
func randInt(n int) (int, error) {
	if n <= 0 {
		return 0, ErrLength
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

// shuffle is Fisher-Yates driven by crypto/rand.
func shuffle(r []rune) error {
	for i := len(r) - 1; i > 0; i-- {
		j, err := randInt(i + 1)
		if err != nil {
			return err
		}
		r[i], r[j] = r[j], r[i]
	}
	return nil
}
