package keys

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// AppKeyEnv is the only accepted source of the Sia app key.
//
// It is never a flag or a tool argument: flags land in shell history and in the
// process table, and a tool argument would let a calling model read or forge it.
const AppKeyEnv = "MNEMOSIA_APP_KEY"

// ErrNoAppKey reports that the environment carries no app key.
var ErrNoAppKey = fmt.Errorf("%s is not set", AppKeyEnv)

// An AppKey authorizes this installation against a Sia indexer. It is issued
// once by an interactive approval and is not derivable from the recovery
// phrase alone.
type AppKey []byte

// AppKeyFromEnv reads and decodes the app key from the environment.
func AppKeyFromEnv() (AppKey, error) {
	raw := strings.TrimSpace(os.Getenv(AppKeyEnv))
	if raw == "" {
		return nil, ErrNoAppKey
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", AppKeyEnv, err)
	}
	if len(key) == 0 {
		return nil, ErrNoAppKey
	}
	return key, nil
}

// Fingerprint identifies an app key in logs and error messages without
// disclosing it.
func (k AppKey) Fingerprint() string {
	if len(k) < 4 {
		return "invalid"
	}
	return hex.EncodeToString(k[:4])
}

// MissingAppKey reports whether err is the absent-app-key case, which callers
// answer with onboarding instructions rather than a stack trace.
func MissingAppKey(err error) bool { return errors.Is(err, ErrNoAppKey) }
