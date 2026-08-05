// Command mnemosia is the command-line interface to a vault.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/steven3002/mnemosia/keys"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "init":
		err = runInit(ctx, os.Args[2:])
	case "remember":
		err = runRemember(ctx, os.Args[2:])
	case "recall":
		err = runRecall(ctx, os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "mnemosia: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		report(err)
		os.Exit(1)
	}
}

// report explains the two failures a first run actually hits, rather than
// printing the underlying error and leaving the reader to work out what to do.
func report(err error) {
	switch {
	case keys.MissingAppKey(err):
		fmt.Fprintf(os.Stderr, "mnemosia: no Sia app key.\n\n"+
			"  Set %s to the app key issued when you approved this\n"+
			"  installation with your indexer. It is a secret: keep it out of\n"+
			"  shell history and never pass it as an argument.\n", keys.AppKeyEnv)
	case errors.Is(err, keys.ErrNoPhrase):
		fmt.Fprintf(os.Stderr, "mnemosia: no recovery phrase.\n\n"+
			"  Set %s, or pipe the phrase in on stdin. The vault derives its\n"+
			"  keys from it on every run and never stores it.\n", keys.PhraseEnv)
	default:
		fmt.Fprintf(os.Stderr, "mnemosia: %v\n", err)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `mnemosia, user-owned storage for an AI's memory, on Sia

usage:
  mnemosia init                      derive keys, prepare the vault, connect
  mnemosia remember "<statement>"    store a memory
  mnemosia recall "<query>"          retrieve memories by meaning

environment:
  MNEMOSIA_PHRASE     BIP-39 recovery phrase; read from stdin when unset
  MNEMOSIA_APP_KEY    Sia app key, issued by indexer approval
  MNEMOSIA_HOME       vault directory (default ~/.mnemosia)
  MNEMOSIA_INDEXER    indexer URL (default https://sia.storage)
  MNEMOSIA_MODEL_DIR  where embedding models are kept

run "mnemosia <command> -h" for the flags of one command.
`)
}
