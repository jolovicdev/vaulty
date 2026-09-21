// Package totp reads the one-time-password settings KeePassXC stores on an
// entry and produces codes from them. The HMAC comes from crypto/hmac and the
// hashes from crypto/sha1, sha256 and sha512; RFC 6238 is implemented here
// only in the sense of reading a counter and truncating a digest.
package totp

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 defines SHA-1 as the default HMAC for TOTP
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNoSeed      = errors.New("totp: no seed")
	ErrBadSeed     = errors.New("totp: seed is not valid base32")
	ErrBadURI      = errors.New("totp: not an otpauth uri")
	ErrUnsupported = errors.New("totp: unsupported algorithm or type")
)

// Config is a parsed TOTP setting.
type Config struct {
	Secret    []byte
	Digits    int
	Period    int
	Algorithm string
	Issuer    string
	Account   string
}

// Defaults per RFC 6238 and what almost every service uses.
const (
	DefaultDigits = 6
	DefaultPeriod = 30
	AlgoSHA1      = "SHA1"
	AlgoSHA256    = "SHA256"
	AlgoSHA512    = "SHA512"
)

// Parse reads the two shapes KeePassXC writes.
//
// The current shape is an otpauth:// URI in the entry's "otp" field. The
// older shape is a bare base32 seed in "TOTP Seed" plus a "TOTP Settings"
// value of "period;digits"; vault.TOTPSeed joins those two with a pipe, which
// is the form this function also accepts.
func Parse(raw string) (Config, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Config{}, ErrNoSeed
	}
	if strings.HasPrefix(strings.ToLower(raw), "otpauth://") {
		return parseURI(raw)
	}
	return parseLegacy(raw)
}

func parseURI(raw string) (Config, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Config{}, ErrBadURI
	}
	if !strings.EqualFold(u.Host, "totp") {
		return Config{}, ErrUnsupported
	}

	q := u.Query()
	cfg := Config{
		Digits:    DefaultDigits,
		Period:    DefaultPeriod,
		Algorithm: AlgoSHA1,
		Issuer:    q.Get("issuer"),
	}

	// The label is "Issuer:Account" or just "Account".
	label := strings.TrimPrefix(u.Path, "/")
	if i := strings.Index(label, ":"); i >= 0 {
		if cfg.Issuer == "" {
			cfg.Issuer = label[:i]
		}
		cfg.Account = strings.TrimSpace(label[i+1:])
	} else {
		cfg.Account = label
	}

	secret, err := decodeBase32(q.Get("secret"))
	if err != nil {
		return Config{}, err
	}
	cfg.Secret = secret

	if v := q.Get("digits"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 6 || n > 10 {
			return Config{}, fmt.Errorf("totp: bad digits %q", v)
		}
		cfg.Digits = n
	}
	if v := q.Get("period"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("totp: bad period %q", v)
		}
		cfg.Period = n
	}
	if v := q.Get("algorithm"); v != "" {
		algo := strings.ToUpper(v)
		if _, err := newHash(algo); err != nil {
			return Config{}, err
		}
		cfg.Algorithm = algo
	}
	return cfg, nil
}

// parseLegacy reads "SEED" or "SEED|period;digits", which is how the older
// KeePassXC fields arrive.
func parseLegacy(raw string) (Config, error) {
	cfg := Config{Digits: DefaultDigits, Period: DefaultPeriod, Algorithm: AlgoSHA1}

	seed := raw
	if i := strings.Index(raw, "|"); i >= 0 {
		seed = raw[:i]
		for j, part := range strings.Split(raw[i+1:], ";") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				continue
			}
			if j == 0 && n > 0 {
				cfg.Period = n
			}
			if j == 1 && n >= 6 && n <= 10 {
				cfg.Digits = n
			}
		}
	}

	secret, err := decodeBase32(seed)
	if err != nil {
		return Config{}, err
	}
	cfg.Secret = secret
	return cfg, nil
}

// decodeBase32 accepts the seed with or without padding and with spaces,
// which is how people paste it off a screen.
func decodeBase32(s string) ([]byte, error) {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(strings.TrimSpace(s)))
	if s == "" {
		return nil, ErrNoSeed
	}
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return nil, ErrBadSeed
	}
	if len(b) == 0 {
		return nil, ErrBadSeed
	}
	return b, nil
}

func newHash(algo string) (func() hash.Hash, error) {
	switch strings.ToUpper(algo) {
	case "", AlgoSHA1:
		return sha1.New, nil
	case AlgoSHA256:
		return sha256.New, nil
	case AlgoSHA512:
		return sha512.New, nil
	}
	return nil, ErrUnsupported
}

// Code returns the code for the time step containing at.
func (c Config) Code(at time.Time) (string, error) {
	h, err := newHash(c.Algorithm)
	if err != nil {
		return "", err
	}
	if len(c.Secret) == 0 {
		return "", ErrNoSeed
	}
	period := c.Period
	if period <= 0 {
		period = DefaultPeriod
	}
	digits := c.Digits
	if digits <= 0 {
		digits = DefaultDigits
	}

	// A time before 1970 has a negative Unix value, which would wrap when
	// converted; RFC 6238 counts steps from the epoch, so clamp to it.
	seconds := at.Unix()
	if seconds < 0 {
		seconds = 0
	}
	counter := uint64(seconds) / uint64(period)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)

	mac := hmac.New(h, c.Secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	// RFC 6238 dynamic truncation: the low nibble of the last byte selects
	// the 4-byte window to read.
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, value%mod), nil
}

// Remaining is how long the code for at stays valid.
func (c Config) Remaining(at time.Time) time.Duration {
	period := c.Period
	if period <= 0 {
		period = DefaultPeriod
	}
	elapsed := at.Unix() % int64(period)
	return time.Duration(int64(period)-elapsed) * time.Second
}
