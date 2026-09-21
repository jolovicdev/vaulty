package strength

import (
	"testing"
)

func testDict() Dictionary {
	// Ranked most common first, which is what the estimator charges by.
	return NewDictionary([]string{
		"password", "dragon", "monkey", "letmein", "hunter", "abacus",
		"trombone", "correct", "horse", "battery", "staple",
	})
}

func TestEstimateBands(t *testing.T) {
	dict := testDict()
	cases := []struct {
		name      string
		password  string
		wantScore int
	}{
		{"empty", "", 0},
		{"single letter", "a", 0},
		{"top dictionary word", "password", 0},
		{"leet dictionary word", "p4ssw0rd", 0},
		{"short digits", "1234", 0},
		{"keyboard run", "qwerty", 0},
		{"repeated character", "aaaaaaaa", 0},
		{"short random", "xK9q", 0},
		{"twelve random lower", "qzmvbtrwnhdl", 2},
		{"sixteen mixed", "qZ9#mVb2TrW7nHdL", 4},
		{"long generated", "8Kq#mV2b$TrW7nHdL!pXz", 4},
		// Six words drawn from a ten word list really is only fair: the
		// estimate follows the dictionary it is given. The same phrase
		// against a realistic list is checked below.
		{"six word passphrase from a tiny list", "abacus-trombone-correct-horse-battery-staple", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Estimate(c.password, dict)
			if got.Score != c.wantScore {
				t.Errorf("Score = %d (%.1f bits, %s), want %d",
					got.Score, got.Entropy, got.Label, c.wantScore)
			}
		})
	}
}

// TestPatternsCostLessThanBruteForce is the property that makes the estimate
// useful: a recognised pattern must score below a random string of the same
// length and alphabet.
func TestPatternsCostLessThanBruteForce(t *testing.T) {
	dict := testDict()
	cases := []struct {
		name    string
		pattern string
		random  string
	}{
		{"dictionary word", "trombone", "xqvbmzlk"},
		{"digit sequence", "12345678", "83921647"},
		{"keyboard run", "asdfghjk", "kdjhsgaf"},
		{"repeat", "mmmmmmmm", "mqxbvtln"},
		{"reversed word", "enobmort", "xqvbmzlk"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Estimate(c.pattern, dict)
			r := Estimate(c.random, dict)
			if p.Entropy >= r.Entropy {
				t.Errorf("%q scored %.1f bits, %q scored %.1f; the pattern must score lower",
					c.pattern, p.Entropy, c.random, r.Entropy)
			}
		})
	}
}

// TestPassphraseFromARealisticListIsStrong uses a dictionary the size of the
// EFF long list, which is what the app actually passes in. Six words from
// 7776 is 77.6 bits before the estimator charges anything for the
// separators.
func TestPassphraseFromARealisticListIsStrong(t *testing.T) {
	words := make([]string, 7776)
	for i := range words {
		words[i] = syntheticWord(i)
	}
	dict := NewDictionary(words)

	phrase := syntheticWord(7000) + "-" + syntheticWord(7100) + "-" + syntheticWord(7200) +
		"-" + syntheticWord(7300) + "-" + syntheticWord(7400) + "-" + syntheticWord(7500)
	got := Estimate(phrase, dict)
	if got.Score != 4 {
		t.Errorf("Score = %d (%.1f bits) for %q, want 4", got.Score, got.Entropy, phrase)
	}
}

// syntheticWord builds a distinct five letter word for index i, avoiding the
// keyboard runs and repeats that real words do not have either.
func syntheticWord(i int) string {
	const letters = "bcdfghjklmnpqrstvwxyz"
	n := len(letters)
	return string([]byte{
		letters[i%n],
		letters[(i/n+3)%n],
		letters[(i/(n*n)+7)%n],
		letters[(i*7+11)%n],
		letters[(i*13+5)%n],
	})
}

// TestLongerIsNeverWeaker walks the prefixes of one fixed string. Adding a
// character can never make a password cheaper to guess, because every piece
// the estimator charges for costs at least one guess.
func TestLongerIsNeverWeaker(t *testing.T) {
	dict := testDict()
	const pw = "qZ9#mVb2TrW7nHdL!pXk"
	last := 0.0
	for i := 1; i <= len(pw); i++ {
		prefix := pw[:i]
		got := Estimate(prefix, dict)
		if got.Entropy < last {
			t.Errorf("%q scored %.1f bits, below its own shorter prefix at %.1f",
				prefix, got.Entropy, last)
		}
		last = got.Entropy
	}
}

func TestWarningNamesThePattern(t *testing.T) {
	dict := testDict()
	cases := []struct {
		password string
		want     string
	}{
		{"password", "contains a dictionary word"},
		{"p4ssw0rd", "contains a dictionary word with letters swapped for symbols"},
		{"drowssap", "contains a reversed dictionary word"},
		{"aaaaaaa", "contains a repeated character"},
		{"123456", "contains a character sequence"},
	}
	for _, c := range cases {
		t.Run(c.password, func(t *testing.T) {
			got := Estimate(c.password, dict)
			if got.Warning != c.want {
				t.Errorf("Warning = %q, want %q", got.Warning, c.want)
			}
		})
	}
}

// TestStrongPasswordHasNoWarning keeps the meter quiet when there is nothing
// to say.
func TestStrongPasswordHasNoWarning(t *testing.T) {
	got := Estimate("8Kq#mV2b$TrW7nHdL!pXz", testDict())
	if got.Warning != "" {
		t.Errorf("Warning = %q, want empty for a strong password", got.Warning)
	}
	if got.Score != 4 {
		t.Errorf("Score = %d, want 4", got.Score)
	}
}

func TestNilDictionaryStillScores(t *testing.T) {
	got := Estimate("password", nil)
	if got.Entropy <= 0 {
		t.Errorf("Entropy = %.1f, want a positive brute force estimate", got.Entropy)
	}
	if got.Warning != "" {
		t.Errorf("Warning = %q, want empty without a dictionary", got.Warning)
	}
}

func TestClassSizeCountsEachClassOnce(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"abc", 26},
		{"ABC", 26},
		{"abcABC", 52},
		{"abc123", 36},
		{"abcABC123!", 95},
		{"é", 100},
	}
	for _, c := range cases {
		if got := classSize(c.in); got != c.want {
			t.Errorf("classSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestMachineIssuedValuesAreNotGraded is the distinction that matters for an
// API key: a score is advice, and there is no advice to give about a value
// the user did not choose and cannot change.
func TestMachineIssuedValuesAreNotGraded(t *testing.T) {
	dict := testDict()
	cases := []struct {
		name       string
		value      string
		wantIssuer string
	}{
		{"github token", "ghp_1l0OI5S2zB8xQwRtYuIoPaSdFgHj", "GitHub personal access token"},
		{"anthropic key", "sk-ant-api03-Xk4pQ2vNb8mZr0tLw7yHjKd", "Anthropic API key"},
		{"aws access key", "AKIAIOSFODNN7EXAMPLE", "AWS access key id"},
		{"slack bot token", "xoxb-1234567890-abcdefghijklmnop", "Slack bot token"},
		{"jwt", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIn0", "JSON Web Token"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Estimate(c.value, dict)
			if !got.Machine {
				t.Errorf("Machine = false for %q", c.value)
			}
			if got.Issuer != c.wantIssuer {
				t.Errorf("Issuer = %q, want %q", got.Issuer, c.wantIssuer)
			}
			if got.Warning != "" {
				t.Errorf("Warning = %q, want none for a machine issued value", got.Warning)
			}
		})
	}
}

// TestSkAntBeatsSk pins the longest-prefix rule: both match, and the more
// specific issuer is the right answer.
func TestSkAntBeatsSk(t *testing.T) {
	got := Estimate("sk-ant-api03-Xk4pQ2vNb8mZr0tLw7yHjKd", testDict())
	if got.Issuer != "Anthropic API key" {
		t.Errorf("Issuer = %q, want the Anthropic key", got.Issuer)
	}
}

// TestShortPrefixIsNotAToken keeps "sk-abc" from being announced as an
// OpenAI key.
func TestShortPrefixIsNotAToken(t *testing.T) {
	got := Estimate("sk-abc", testDict())
	if got.Issuer != "" {
		t.Errorf("Issuer = %q for a six character value", got.Issuer)
	}
	if got.Machine {
		t.Error("Machine = true for a six character value")
	}
}

// TestUnrecognisedRandomValueIsStillMachine covers a key from a service that
// is not in the prefix list: the pattern search finds nothing human in it,
// so it is reported as random rather than praised.
func TestUnrecognisedRandomValueIsStillMachine(t *testing.T) {
	got := Estimate("7Kq9zXm2Vb4nPt6wRy8uHj3dFg5sLc1a", testDict())
	if !got.Machine {
		t.Fatalf("Machine = false (%.0f bits, %q)", got.Entropy, got.Label)
	}
	if got.Issuer != "" {
		t.Errorf("Issuer = %q, want empty for an unrecognised format", got.Issuer)
	}
	if got.Label != "random" {
		t.Errorf("Label = %q, want random", got.Label)
	}
}

// TestChosenPasswordsAreStillGraded is the other half: a value with a human
// pattern in it keeps its band and its warning, because there the advice is
// worth something.
func TestChosenPasswordsAreStillGraded(t *testing.T) {
	dict := testDict()
	cases := []struct {
		value       string
		wantMachine bool
		wantWarning bool
	}{
		{"password", false, true},
		{"dragon123", false, true},
		{"qwerty", false, true},
		{"Tr0mbone-Monkey", false, true},
		{"hunter2", false, true},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			got := Estimate(c.value, dict)
			if got.Machine != c.wantMachine {
				t.Errorf("Machine = %v, want %v", got.Machine, c.wantMachine)
			}
			if (got.Warning != "") != c.wantWarning {
				t.Errorf("Warning = %q, want a warning: %v", got.Warning, c.wantWarning)
			}
		})
	}
}
