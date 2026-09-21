package generator

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordRespectsOptions(t *testing.T) {
	cases := []struct {
		name  string
		opts  PasswordOptions
		check func(t *testing.T, got string)
	}{
		{
			name: "lower only",
			opts: PasswordOptions{Length: 32, Lower: true},
			check: func(t *testing.T, got string) {
				assertOnlyFrom(t, got, Lower)
			},
		},
		{
			name: "digits only",
			opts: PasswordOptions{Length: 16, Digits: true},
			check: func(t *testing.T, got string) {
				assertOnlyFrom(t, got, Digits)
			},
		},
		{
			name: "all classes appear",
			opts: PasswordOptions{Length: 24, Lower: true, Upper: true, Digits: true, Symbols: true},
			check: func(t *testing.T, got string) {
				for name, class := range map[string]string{
					"lower": Lower, "upper": Upper, "digits": Digits, "symbols": Symbols,
				} {
					if !strings.ContainsAny(got, class) {
						t.Errorf("no %s character in %q", name, got)
					}
				}
			},
		},
		{
			name: "ambiguous excluded",
			opts: PasswordOptions{Length: 64, Lower: true, Upper: true, Digits: true, ExcludeAmbiguous: true},
			check: func(t *testing.T, got string) {
				for _, r := range "0O1lI5S2Z8B" {
					if strings.ContainsRune(got, r) {
						t.Errorf("ambiguous character %q in %q", r, got)
					}
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Password(c.opts)
			if err != nil {
				t.Fatal(err)
			}
			if len([]rune(got)) != c.opts.Length {
				t.Fatalf("length = %d, want %d", len([]rune(got)), c.opts.Length)
			}
			c.check(t, got)
		})
	}
}

func TestPasswordErrors(t *testing.T) {
	cases := []struct {
		name string
		opts PasswordOptions
		want error
	}{
		{"no classes", PasswordOptions{Length: 16}, ErrNoCharacters},
		{"too short", PasswordOptions{Length: 1, Lower: true}, ErrLength},
		{"too long", PasswordOptions{Length: MaxLength + 1, Lower: true}, ErrLength},
		{"more classes than length", PasswordOptions{Length: 4, Lower: true, Upper: true, Digits: true, Symbols: true}, nil},
		{"fewer slots than classes", PasswordOptions{Length: 3, Lower: true, Upper: true, Digits: true, Symbols: true}, ErrLength},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Password(c.opts)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

// TestPasswordVaries is a smoke test against a generator that forgot to draw
// fresh randomness: 200 draws of a 20 character password must all differ.
func TestPasswordVaries(t *testing.T) {
	opts := DefaultPasswordOptions()
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		got, err := Password(opts)
		if err != nil {
			t.Fatal(err)
		}
		if seen[got] {
			t.Fatalf("duplicate password after %d draws: %q", i, got)
		}
		seen[got] = true
	}
}

func TestPassphrase(t *testing.T) {
	cases := []struct {
		name  string
		opts  PassphraseOptions
		check func(t *testing.T, got string)
	}{
		{
			name: "word count and separator",
			opts: PassphraseOptions{Words: 6, Separator: "-"},
			check: func(t *testing.T, got string) {
				if n := len(strings.Split(got, "-")); n != 6 {
					t.Errorf("got %d words in %q, want 6", n, got)
				}
				if got != strings.ToLower(got) {
					t.Errorf("%q is not lower case", got)
				}
			},
		},
		{
			name: "capitalised",
			opts: PassphraseOptions{Words: 4, Separator: ".", Capitalize: true},
			check: func(t *testing.T, got string) {
				for _, word := range strings.Split(got, ".") {
					if word == "" || word[0] < 'A' || word[0] > 'Z' {
						t.Errorf("word %q is not capitalised in %q", word, got)
					}
				}
			},
		},
		{
			name: "trailing number",
			opts: PassphraseOptions{Words: 3, Separator: "_", AddNumber: true},
			check: func(t *testing.T, got string) {
				parts := strings.Split(got, "_")
				if len(parts) != 4 {
					t.Fatalf("got %d parts in %q, want 4", len(parts), got)
				}
				if last := parts[3]; len(last) != 1 || last[0] < '0' || last[0] > '9' {
					t.Errorf("last part %q is not a digit", last)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Passphrase(c.opts)
			if err != nil {
				t.Fatal(err)
			}
			c.check(t, got)
		})
	}
}

func TestPassphraseErrors(t *testing.T) {
	for _, n := range []int{0, MinWords - 1, MaxWords + 1} {
		if _, err := Passphrase(PassphraseOptions{Words: n, Separator: "-"}); !errors.Is(err, ErrLength) {
			t.Errorf("Words=%d: err = %v, want ErrLength", n, err)
		}
	}
}

// TestWordlistIsTheEFFLongList pins the embedded list: 7776 entries is what
// makes each word worth 12.925 bits.
func TestWordlistIsTheEFFLongList(t *testing.T) {
	list, err := Words()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 7776 {
		t.Fatalf("wordlist has %d entries, want 7776", len(list))
	}
	if list[0] != "abacus" {
		t.Errorf("first word = %q, want abacus", list[0])
	}
	if last := list[len(list)-1]; last != "zoom" {
		t.Errorf("last word = %q, want zoom", last)
	}
	for i, w := range list {
		if w != strings.ToLower(w) || strings.ContainsAny(w, " \t") {
			t.Fatalf("entry %d is not a bare lower case word: %q", i, w)
		}
	}
}

func assertOnlyFrom(t *testing.T, got, allowed string) {
	t.Helper()
	for _, r := range got {
		if !strings.ContainsRune(allowed, r) {
			t.Errorf("character %q in %q is outside the selected alphabet", r, got)
		}
	}
}
