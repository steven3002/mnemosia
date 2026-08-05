package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/steven3002/mnemosia/recall"
)

func runRecall(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("recall", flag.ExitOnError)
	var flags vaultFlags
	flags.bind(fs)
	limit := fs.Int("limit", recall.DefaultLimit, "how many records to return")
	fromNetwork := fs.Bool("from-network", false,
		"read bodies from Sia even when this device holds a copy, to check the stored data")
	if err := fs.Parse(args); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return fmt.Errorf("nothing to recall: pass the query as an argument")
	}

	v, err := flags.open(ctx)
	if err != nil {
		return err
	}
	defer v.Close()

	result, err := v.Recall(ctx, recall.Request{Query: query, Limit: *limit})
	if err != nil {
		return err
	}

	fmt.Fprintf(stderr, "  embed %s · search %s over %d vector(s) · fetch %s\n",
		took(result.EmbedFor), took(result.SearchFor), result.Searched, took(result.FetchFor))

	// No results is an answer, not a failure. A memory store that returns its
	// least-bad guess when it holds nothing relevant is worse than one that
	// says so.
	if len(result.Hits) == 0 {
		fmt.Fprintln(stderr, "  no records matched")
		return nil
	}

	for n, hit := range result.Hits {
		memory, source, elapsed := hit.Memory, string(hit.Tier), hit.FetchedIn
		if *fromNetwork {
			start := time.Now()
			fetched, err := v.FetchFromNetwork(ctx, memory.ID)
			if err != nil {
				return err
			}
			memory, source, elapsed = fetched, "network", time.Since(start)
		}
		fmt.Printf("%d. [%.4f] %s\n", n+1, hit.Score, memory.Statement)
		if memory.Context != "" {
			fmt.Printf("   context: %s\n", memory.Context)
		}
		fmt.Printf("   %s · %s", memory.ID, memory.Type)
		if len(memory.Tags) > 0 {
			fmt.Printf(" · %s", strings.Join(memory.Tags, ", "))
		}
		fmt.Printf(" · from %s in %s\n", source, took(elapsed))
	}
	return nil
}
