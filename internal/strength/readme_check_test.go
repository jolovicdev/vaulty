package strength

import (
	"math"
	"testing"
)

// TestReadmeExamples pins the three values the README quotes. Documentation
// that states a number has to be checkable, or it drifts.
func TestReadmeExamples(t *testing.T) {
	// A realistic dictionary, since the README describes the behaviour
	// against the list the app embeds rather than a ten word stub.
	words := make([]string, 7776)
	for i := range words {
		words[i] = syntheticWord(i)
	}
	dict := NewDictionary(append(words, "trombone", "monkey"))

	cases := []struct {
		value       string
		wantMachine bool
		wantIssuer  string
		wantBits    int
		wantWarning string
	}{
		{"ghp_1l0OI5S2zB8xQwRtYuIoPaSdFg", true, "GitHub personal access token", 197, ""},
		{"7Kq9zXm2Vb4nPt6wRy8uHj3dFg5sLc1a", true, "", 181, ""},
		{"Tr0mbone-Monkey", false, "", 0, "contains a dictionary word with letters swapped for symbols"},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			got := Estimate(c.value, dict)
			if got.Machine != c.wantMachine {
				t.Errorf("Machine = %v, want %v", got.Machine, c.wantMachine)
			}
			if got.Issuer != c.wantIssuer {
				t.Errorf("Issuer = %q, want %q", got.Issuer, c.wantIssuer)
			}
			// Round rather than truncate, and print the unrounded value:
			// comparing int(180.7) against 181 fails while printing "181,
			// want 181", which is a message that lies about its own test.
			if c.wantBits > 0 && int(math.Round(got.Entropy)) != c.wantBits {
				t.Errorf("Entropy = %.1f bits, want %d as the README states", got.Entropy, c.wantBits)
			}
			if c.wantWarning != "" && got.Warning != c.wantWarning {
				t.Errorf("Warning = %q, want %q", got.Warning, c.wantWarning)
			}
		})
	}
}
