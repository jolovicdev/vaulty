package totp

import (
	"errors"
	"testing"
	"time"
)

// RFC 6238 appendix B test vectors. The seed is the ASCII string
// "12345678901234567890" base32 encoded, which is what the RFC uses.
const rfcSeed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestRFC6238Vectors(t *testing.T) {
	cases := []struct {
		unix int64
		algo string
		want string
	}{
		{59, AlgoSHA1, "94287082"},
		{1111111109, AlgoSHA1, "07081804"},
		{1111111111, AlgoSHA1, "14050471"},
		{1234567890, AlgoSHA1, "89005924"},
		{2000000000, AlgoSHA1, "69279037"},
		{20000000000, AlgoSHA1, "65353130"},
	}
	for _, c := range cases {
		t.Run(time.Unix(c.unix, 0).UTC().Format(time.RFC3339), func(t *testing.T) {
			cfg, err := Parse(rfcSeed)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Digits = 8
			cfg.Algorithm = c.algo
			got, err := cfg.Code(time.Unix(c.unix, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("Code = %s, want %s", got, c.want)
			}
		})
	}
}

func TestParseOtpauthURI(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want Config
	}{
		{
			name: "keepassxc default",
			raw:  "otpauth://totp/GitHub:octocat?secret=JBSWY3DPEHPK3PXP&issuer=GitHub",
			want: Config{Digits: 6, Period: 30, Algorithm: AlgoSHA1, Issuer: "GitHub", Account: "octocat"},
		},
		{
			name: "explicit parameters",
			raw:  "otpauth://totp/Example:bob?secret=JBSWY3DPEHPK3PXP&digits=8&period=60&algorithm=SHA256",
			want: Config{Digits: 8, Period: 60, Algorithm: AlgoSHA256, Issuer: "Example", Account: "bob"},
		},
		{
			name: "label without an issuer",
			raw:  "otpauth://totp/plainlabel?secret=JBSWY3DPEHPK3PXP",
			want: Config{Digits: 6, Period: 30, Algorithm: AlgoSHA1, Account: "plainlabel"},
		},
		{
			name: "url encoded label",
			raw:  "otpauth://totp/My%20Site:a%40b.com?secret=JBSWY3DPEHPK3PXP",
			want: Config{Digits: 6, Period: 30, Algorithm: AlgoSHA1, Issuer: "My Site", Account: "a@b.com"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(c.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Digits != c.want.Digits {
				t.Errorf("Digits = %d, want %d", got.Digits, c.want.Digits)
			}
			if got.Period != c.want.Period {
				t.Errorf("Period = %d, want %d", got.Period, c.want.Period)
			}
			if got.Algorithm != c.want.Algorithm {
				t.Errorf("Algorithm = %q, want %q", got.Algorithm, c.want.Algorithm)
			}
			if got.Issuer != c.want.Issuer {
				t.Errorf("Issuer = %q, want %q", got.Issuer, c.want.Issuer)
			}
			if got.Account != c.want.Account {
				t.Errorf("Account = %q, want %q", got.Account, c.want.Account)
			}
			if len(got.Secret) == 0 {
				t.Error("Secret is empty")
			}
		})
	}
}

// TestParseLegacyKeePassXCFields covers the older storage shape, where the
// seed and its settings live in two separate entry fields.
func TestParseLegacyKeePassXCFields(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantDigits int
		wantPeriod int
	}{
		{"bare seed", "JBSWY3DPEHPK3PXP", 6, 30},
		{"seed with settings", "JBSWY3DPEHPK3PXP|30;6", 6, 30},
		{"steam style settings", "JBSWY3DPEHPK3PXP|60;8", 8, 60},
		{"seed with spaces", "JBSW Y3DP EHPK 3PXP", 6, 30},
		{"lower case seed", "jbswy3dpehpk3pxp", 6, 30},
		{"padded seed", "JBSWY3DPEHPK3PXP======", 6, 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(c.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Digits != c.wantDigits {
				t.Errorf("Digits = %d, want %d", got.Digits, c.wantDigits)
			}
			if got.Period != c.wantPeriod {
				t.Errorf("Period = %d, want %d", got.Period, c.wantPeriod)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{"empty", "", ErrNoSeed},
		{"whitespace", "   ", ErrNoSeed},
		{"not base32", "not-base-32!!", ErrBadSeed},
		{"hotp is not supported", "otpauth://hotp/X?secret=JBSWY3DPEHPK3PXP", ErrUnsupported},
		{"no secret in uri", "otpauth://totp/X?issuer=Y", ErrNoSeed},
		{"unknown algorithm", "otpauth://totp/X?secret=JBSWY3DPEHPK3PXP&algorithm=MD5", ErrUnsupported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.raw)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestCodeIsStableWithinAPeriodAndChangesAcross(t *testing.T) {
	cfg, err := Parse("otpauth://totp/X?secret=JBSWY3DPEHPK3PXP&period=30")
	if err != nil {
		t.Fatal(err)
	}

	// 1700000010 is exactly divisible by 30, so it is the first second of a
	// window and the last second of that window is 29 seconds later.
	start := time.Unix(1_700_000_010, 0).UTC()
	a, err := cfg.Code(start)
	if err != nil {
		t.Fatal(err)
	}
	b, err := cfg.Code(start.Add(29 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("code changed inside one period: %s then %s", a, b)
	}

	c, err := cfg.Code(start.Add(30 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Errorf("code did not change across the period boundary: %s", a)
	}
}

func TestRemaining(t *testing.T) {
	cfg := Config{Period: 30}
	cases := []struct {
		unix int64
		want time.Duration
	}{
		{1_700_000_000, 10 * time.Second},
		{1_700_000_010, 30 * time.Second},
		{1_700_000_029, 11 * time.Second},
	}
	for _, c := range cases {
		got := cfg.Remaining(time.Unix(c.unix, 0).UTC())
		if got != c.want {
			t.Errorf("Remaining(%d) = %v, want %v", c.unix, got, c.want)
		}
	}
}
