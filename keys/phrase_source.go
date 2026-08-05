package keys

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// PhraseEnv is the environment variable carrying the recovery phrase.
const PhraseEnv = "MNEMOSIA_PHRASE"

// ErrNoPhrase reports that no recovery phrase was supplied.
var ErrNoPhrase = fmt.Errorf("no recovery phrase: set %s or pipe it on stdin", PhraseEnv)

// ReadPhrase takes the recovery phrase from the environment, falling back to
// stdin. The vault never persists it: holding it only for the lifetime of one
// command is what makes "keys never leave the device" enforceable rather than
// aspirational.
func ReadPhrase(stdin io.Reader) (string, error) {
	if phrase := strings.TrimSpace(os.Getenv(PhraseEnv)); phrase != "" {
		return phrase, nil
	}
	if stdin == nil {
		return "", ErrNoPhrase
	}
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read recovery phrase: %w", err)
		}
		return "", ErrNoPhrase
	}
	phrase := strings.TrimSpace(scanner.Text())
	if phrase == "" {
		return "", ErrNoPhrase
	}
	return phrase, nil
}
