package vault

import (
	"context"
	"fmt"
	"time"

	"github.com/steven3002/mnemosia/manifest"
	"github.com/steven3002/mnemosia/record"
	"github.com/steven3002/mnemosia/seal"
	"github.com/steven3002/mnemosia/store"
	"github.com/steven3002/mnemosia/store/packer"
)

// A RememberRequest is one memory to store.
type RememberRequest struct {
	Statement string
	Context   string
	Type      record.Type
	Tags      []string
	Source    record.Source
}

// A RememberResult reports what was stored.
type RememberResult struct {
	ID     record.ID
	CID    seal.CID
	Stored record.Time
	// OnNetwork reports whether the record reached Sia before this call
	// returned. When it is false the record is durable on this device only, and
	// nothing may present that as being stored on the network.
	OnNetwork bool
	// Flushed describes the write, when one happened.
	Flushed *store.Flush

	// EmbedFor, SealFor and FlushFor are what the write actually spent.
	EmbedFor time.Duration
	SealFor  time.Duration
	FlushFor time.Duration
}

// Remember stores a memory.
//
// Validation, embedding and the write to this device all complete before the
// call returns; reaching the network is queued. That ordering is what makes the
// user's wait independent of the network, and it is also why the result says
// plainly whether the record has left the device yet.
func (v *Vault) Remember(ctx context.Context, req RememberRequest) (RememberResult, error) {
	id, err := record.NewID()
	if err != nil {
		return RememberResult{}, err
	}
	now := record.Now()
	memory := &record.Memory{
		ID:            id,
		Kind:          record.KindMemory,
		Type:          req.Type,
		SchemaVersion: record.SchemaVersion,
		Version:       1,
		Statement:     req.Statement,
		Context:       req.Context,
		Tags:          req.Tags,
		CreatedAt:     now,
		UpdatedAt:     now,
		Source:        req.Source,
		Embedding: record.Embedding{
			Model: v.opts.Model.Name,
			Dim:   v.opts.Model.Dim,
		},
	}
	if err := memory.Validate(); err != nil {
		return RememberResult{}, err
	}

	start := time.Now()
	vector, err := v.embedder.EmbedOne(ctx, memory.IndexText())
	if err != nil {
		return RememberResult{}, err
	}
	result := RememberResult{ID: id, Stored: now, EmbedFor: time.Since(start)}

	body, err := record.Marshal(memory)
	if err != nil {
		return RememberResult{}, err
	}

	start = time.Now()
	cid, err := v.sealer.CID(body)
	if err != nil {
		return RememberResult{}, err
	}
	sealed, err := v.sealer.Seal(body)
	if err != nil {
		return RememberResult{}, err
	}
	framed, err := seal.FrameRecord(record.KindMemory, id, sealed)
	if err != nil {
		return RememberResult{}, err
	}
	result.SealFor = time.Since(start)
	result.CID = cid

	if err := v.local.PutBody(id, record.KindMemory, body); err != nil {
		return RememberResult{}, err
	}
	if err := v.local.PutVector(id, v.opts.Model.Name, vector); err != nil {
		return RememberResult{}, err
	}
	if err := v.index.Add(id, vector); err != nil {
		return RememberResult{}, err
	}

	start = time.Now()
	flushed, err := v.packer.Add(ctx, packer.Queued{
		ID:   id,
		Kind: record.KindMemory,
		Blob: store.Blob{CID: cid.String(), Payload: framed},
	})
	if err != nil {
		return RememberResult{}, err
	}
	result.FlushFor = time.Since(start)

	for _, written := range flushed.Flush.Written {
		if written.CID == cid.String() {
			result.OnNetwork = true
			result.Flushed = &flushed.Flush
			break
		}
	}
	return result, nil
}

// Flush writes everything queued to the network.
func (v *Vault) Flush(ctx context.Context) (*store.Flush, error) {
	result, err := v.packer.Flush(ctx)
	if err != nil {
		return nil, err
	}
	if result.Empty() {
		return nil, nil
	}
	return &result.Flush, nil
}

// recordFlush catalogs a completed flush.
//
// The catalog entry and the slab ledger are written here rather than at queue
// time because neither is knowable until the network answers: an object ref
// only exists once the write lands, and a slab only becomes this vault's
// responsibility once something of ours is pinned in it.
func (v *Vault) recordFlush(result packer.Result) error {
	byCID := make(map[string]store.Written, len(result.Flush.Written))
	for _, written := range result.Flush.Written {
		byCID[written.CID] = written
	}
	for _, queued := range result.Records {
		written, ok := byCID[queued.Blob.CID]
		if !ok {
			return fmt.Errorf("catalog %s: the flush reported no object for it", queued.ID)
		}
		cid, err := seal.ParseCID(queued.Blob.CID)
		if err != nil {
			return fmt.Errorf("catalog %s: %w", queued.ID, err)
		}
		if err := v.manifest.Append(manifest.Entry{
			ID:        queued.ID,
			Kind:      queued.Kind,
			Version:   1,
			CID:       cid,
			ObjectRef: written.ObjectRef.String(),
			SlabID:    string(written.SlabID),
			Bytes:     written.Bytes,
			WrittenAt: record.Now(),
		}); err != nil {
			return fmt.Errorf("catalog %s: %w", queued.ID, err)
		}
	}

	for _, slabID := range result.Flush.Slabs {
		if err := v.reclaimer.Track(slabID, len(result.Records), int64(result.Flush.Bytes())); err != nil {
			return fmt.Errorf("track slab %s: %w", slabID, err)
		}
	}
	return nil
}
