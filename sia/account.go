package sia

import (
	"context"
	"fmt"
	"time"
)

// An Account is the vault's standing with the indexer.
type Account struct {
	// Ready reports whether enough host accounts are funded to write. Approval
	// completes before funding does, so the first write has to wait for this
	// rather than assume it.
	Ready bool
	// PinnedData is quota consumed by data shards; PinnedSize includes parity.
	PinnedData uint64
	PinnedSize uint64
	// MaxPinnedData is the tier's ceiling.
	MaxPinnedData uint64
}

// Free reports the remaining quota.
func (a Account) Free() uint64 {
	if a.PinnedData >= a.MaxPinnedData {
		return 0
	}
	return a.MaxPinnedData - a.PinnedData
}

// Account reads the current account state.
func (c *Client) Account(ctx context.Context) (Account, error) {
	resp, err := c.sdk.Account(ctx)
	if err != nil {
		return Account{}, asIndexerError("read the account", c.indexer, err)
	}
	return Account{
		Ready:         resp.Ready,
		PinnedData:    resp.PinnedData,
		PinnedSize:    resp.PinnedSize,
		MaxPinnedData: resp.MaxPinnedData,
	}, nil
}

// WaitReady blocks until the indexer has funded enough host accounts to accept
// a write, or the budget expires.
func (c *Client) WaitReady(ctx context.Context, budget time.Duration) (Account, error) {
	deadline := time.Now().Add(budget)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		account, err := c.Account(ctx)
		if err != nil {
			return Account{}, err
		}
		if account.Ready {
			return account, nil
		}
		if time.Now().After(deadline) {
			return account, fmt.Errorf("account not ready after %s: the indexer is still funding host accounts", budget)
		}
		select {
		case <-ctx.Done():
			return Account{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
