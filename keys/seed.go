// Package keys derives a vault's keys from its recovery phrase and reads the
// Sia app key from the environment.
package keys

import (
	"errors"
	"fmt"
	"strings"

	"go.sia.tech/coreutils/wallet"
)

// SeedSize is the length of a vault seed in bytes.
const SeedSize = 32

// ErrPhrase is the class of every recovery-phrase failure.
var ErrPhrase = errors.New("recovery phrase")

// A Seed is the root secret of a vault. Every other key in the process is
// derived from it, and it never leaves the device.
type Seed [SeedSize]byte

// NewPhrase returns a fresh BIP-39 recovery phrase.
func NewPhrase() string { return wallet.NewSeedPhrase() }

// SeedFromPhrase derives the vault seed from a BIP-39 recovery phrase.
//
// The same phrase also derives the Sia app key, but only in combination with a
// secret the indexer issues at approval time, so the phrase alone cannot
// reconstruct access to the network, a new device still needs an approval
// round.
func SeedFromPhrase(phrase string) (Seed, error) {
	phrase = strings.Join(strings.Fields(phrase), " ")
	if phrase == "" {
		return Seed{}, fmt.Errorf("%w: empty", ErrPhrase)
	}
	var seed Seed
	if err := wallet.SeedFromPhrase((*[SeedSize]byte)(&seed), phrase); err != nil {
		return Seed{}, fmt.Errorf("%w: %w", ErrPhrase, err)
	}
	return seed, nil
}

// Wipe clears the seed in place. Callers hold it only for as long as they need
// to derive from it.
func (s *Seed) Wipe() { clear(s[:]) }
