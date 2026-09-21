package vault

import (
	"time"

	"github.com/jolovicdev/vaulty/internal/totp"
)

// totpCode is the bridge the interop test uses to compare this project's TOTP
// output against KeePassXC's.
func totpCode(seed string) (string, error) {
	cfg, err := totp.Parse(seed)
	if err != nil {
		return "", err
	}
	return cfg.Code(time.Now())
}
