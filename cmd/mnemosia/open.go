package main

import (
	"context"
	"flag"
	"os"

	"github.com/steven3002/mnemosia/keys"
	"github.com/steven3002/mnemosia/vault"
)

// vaultFlags are the options every command shares.
type vaultFlags struct {
	home    string
	indexer string
	offline bool
}

func (f *vaultFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&f.home, "home", vault.DefaultHome(), "vault directory")
	fs.StringVar(&f.indexer, "indexer", vault.DefaultIndexer(), "indexer URL")
	fs.BoolVar(&f.offline, "offline", false, "work from this device only, without contacting the indexer")
}

// open prepares a vault from the flags and the environment.
//
// Neither secret is a flag. A recovery phrase or an app key passed as an
// argument is visible in the process table and lands in shell history, which
// would make every other precaution in the design decorative.
func (f *vaultFlags) open(ctx context.Context) (*vault.Vault, error) {
	phrase, err := keys.ReadPhrase(os.Stdin)
	if err != nil {
		return nil, err
	}
	opts := vault.Options{
		Home:    f.home,
		Phrase:  phrase,
		Indexer: f.indexer,
		Offline: f.offline,
	}
	if !f.offline {
		appKey, err := keys.AppKeyFromEnv()
		if err != nil {
			return nil, err
		}
		opts.AppKey = appKey
	}
	return vault.Open(ctx, opts)
}
