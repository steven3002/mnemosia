package vault

import (
	"context"
	"errors"
	"fmt"

	"github.com/steven3002/mnemosia/local"
	"github.com/steven3002/mnemosia/manifest"
	"github.com/steven3002/mnemosia/recall"
	"github.com/steven3002/mnemosia/record"
	"github.com/steven3002/mnemosia/seal"
	"github.com/steven3002/mnemosia/sia"
)

// Fetch resolves a record id to its decrypted body, cheapest source first.
//
// The order is not a cache hierarchy bolted on afterwards; the three sources
// cost about an order of magnitude apart, and which one answered is the main
// thing that explains a read's latency.
func (v *Vault) Fetch(ctx context.Context, id record.ID) (*record.Memory, recall.Tier, error) {
	if body, err := v.local.GetBody(id); err == nil {
		memory, err := record.Unmarshal(body)
		return memory, recall.TierLocal, err
	} else if !errors.Is(err, local.ErrNotFound) {
		return nil, "", err
	}

	entry, err := v.manifest.Lookup(id)
	if err != nil {
		return nil, "", err
	}
	if v.client == nil {
		return nil, "", fmt.Errorf("%s is not held on this device: %w", id, errOffline)
	}
	ref, err := sia.ParseObjectRef(entry.ObjectRef)
	if err != nil {
		return nil, "", err
	}

	payload, tier, err := v.fetchRemote(ctx, entry, ref)
	if err != nil {
		return nil, "", err
	}
	memory, err := v.openPayload(id, payload)
	if err != nil {
		return nil, "", err
	}
	// A record fetched from the network is now held here, so the next read of
	// it is free.
	body, err := record.Marshal(memory)
	if err != nil {
		return nil, "", err
	}
	if err := v.local.PutBody(id, entry.Kind, body); err != nil {
		return nil, "", err
	}
	return memory, tier, nil
}

// fetchRemote reads a record's bytes from the network, using cached location
// metadata when it is fresh enough to trust.
func (v *Vault) fetchRemote(ctx context.Context, entry manifest.Entry, ref sia.ObjectRef) ([]byte, recall.Tier, error) {
	cached, err := v.local.GetSlabMeta(entry.ObjectRef)
	switch {
	case err == nil && cached.Age() < v.opts.SlabMetaTTL:
		payload, err := v.client.DownloadCached(sia.SlabMeta{Ref: ref, Bytes: cached.Meta})
		if err == nil {
			return payload, recall.TierCached, nil
		}
		// Fall through to the indexer: cached metadata that no longer works is
		// exactly the case the expiry exists for.
	case err != nil && !errors.Is(err, local.ErrNotFound):
		return nil, "", err
	}

	payload, err := v.client.Download(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	if meta, err := v.client.SlabMetaFor(ctx, ref); err == nil {
		if err := v.local.PutSlabMeta(entry.ObjectRef, entry.SlabID, meta.Bytes); err != nil {
			return nil, "", err
		}
	}
	return payload, recall.TierNetwork, nil
}

// openBody unframes and decrypts one stored record, returning the exact bytes
// that were sealed.
func (v *Vault) openBody(id record.ID, payload []byte) ([]byte, error) {
	frame, _, err := seal.Unframe(payload)
	if err != nil {
		return nil, fmt.Errorf("record %s: %w", id, err)
	}
	if frame.ID != id {
		return nil, fmt.Errorf("record %s: the stored blob identifies itself as %s", id, frame.ID)
	}
	body, err := v.sealer.Open(frame.Sealed)
	if err != nil {
		return nil, fmt.Errorf("record %s: %w", id, err)
	}
	return body, nil
}

// openPayload unframes, decrypts and parses one stored record.
func (v *Vault) openPayload(id record.ID, payload []byte) (*record.Memory, error) {
	body, err := v.openBody(id, payload)
	if err != nil {
		return nil, err
	}
	return record.Unmarshal(body)
}

// FetchFromNetwork reads a record from Sia, bypassing this device's copy.
//
// It exists so a round trip can be checked against what was actually stored
// rather than against what was written locally a moment earlier, the two are
// only the same if the whole path works.
func (v *Vault) FetchFromNetwork(ctx context.Context, id record.ID) (*record.Memory, error) {
	if v.client == nil {
		return nil, errOffline
	}
	entry, err := v.manifest.Lookup(id)
	if err != nil {
		return nil, err
	}
	ref, err := sia.ParseObjectRef(entry.ObjectRef)
	if err != nil {
		return nil, err
	}
	payload, err := v.client.Download(ctx, ref)
	if err != nil {
		return nil, err
	}
	return v.openPayload(id, payload)
}

// StoredBytes returns a record's bytes exactly as they sit on the network,
// still encrypted, for confirming that what left the device is unreadable.
func (v *Vault) StoredBytes(ctx context.Context, id record.ID) ([]byte, error) {
	if v.client == nil {
		return nil, errOffline
	}
	entry, err := v.manifest.Lookup(id)
	if err != nil {
		return nil, err
	}
	ref, err := sia.ParseObjectRef(entry.ObjectRef)
	if err != nil {
		return nil, err
	}
	return v.client.Download(ctx, ref)
}

// BodyFromNetwork returns a record's decrypted bytes as recovered from the
// network, so a round trip can be compared byte for byte rather than field by
// field.
func (v *Vault) BodyFromNetwork(ctx context.Context, id record.ID) ([]byte, error) {
	payload, err := v.StoredBytes(ctx, id)
	if err != nil {
		return nil, err
	}
	return v.openBody(id, payload)
}

// LocalBody returns a record's bytes as this device holds them.
func (v *Vault) LocalBody(id record.ID) ([]byte, error) { return v.local.GetBody(id) }
