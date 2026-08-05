package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/steven3002/mnemosia/record"
	"github.com/steven3002/mnemosia/vault"
)

func runRemember(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("remember", flag.ExitOnError)
	var flags vaultFlags
	flags.bind(fs)
	context_ := fs.String("context", "", "what makes the statement resolvable on its own")
	memType := fs.String("type", string(record.TypeFact),
		"one of "+strings.Join(record.TypeNames(), ", "))
	tags := fs.String("tags", "", "comma-separated tags")
	flush := fs.Bool("flush", true, "write to Sia before returning instead of leaving the record queued")
	if err := fs.Parse(args); err != nil {
		return err
	}
	statement := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if statement == "" {
		return fmt.Errorf("nothing to remember: pass the statement as an argument")
	}

	v, err := flags.open(ctx)
	if err != nil {
		return err
	}
	defer v.Close()

	result, err := v.Remember(ctx, vault.RememberRequest{
		Statement: statement,
		Context:   *context_,
		Type:      record.Type(*memType),
		Tags:      splitTags(*tags),
		Source:    record.Source{Origin: "cli", Client: "mnemosia"},
	})
	if err != nil {
		return err
	}

	if *flush && !result.OnNetwork && v.Online() {
		flushed, err := v.Flush(ctx)
		if err != nil {
			return err
		}
		if flushed != nil {
			result.OnNetwork = true
			result.Flushed = flushed
		}
	}

	fmt.Println(result.ID)
	fmt.Fprintf(stderr, "  cid       %s\n", result.CID)
	fmt.Fprintf(stderr, "  embed     %s\n", took(result.EmbedFor))
	fmt.Fprintf(stderr, "  seal      %s\n", took(result.SealFor))
	if result.OnNetwork && result.Flushed != nil {
		fmt.Fprintf(stderr, "  on Sia    %s in %d object(s), %d slab(s)\n",
			humanBytes(uint64(result.Flushed.Bytes())), len(result.Flushed.Written), len(result.Flushed.Slabs))
		fmt.Fprintf(stderr, "            upload %s · pin slabs %s · pin objects %s\n",
			took(result.Flushed.UploadFor), took(result.Flushed.PinSlabsFor), took(result.Flushed.PinObjectFor))
		fmt.Fprintf(stderr, "  object    %s\n", result.Flushed.Written[0].ObjectRef)
	} else {
		// Saying "saved" without this distinction would be the one dishonest
		// thing this interface could do: until a flush completes the record
		// exists on this device only.
		fmt.Fprintf(stderr, "  on Sia    not yet, held on this device, %d record(s) queued\n", v.Pending())
	}
	return nil
}

func splitTags(raw string) []string {
	var out []string
	for _, tag := range strings.Split(raw, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}
