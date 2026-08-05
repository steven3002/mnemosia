package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/steven3002/mnemosia/embed"
	"github.com/steven3002/mnemosia/index"
	"github.com/steven3002/mnemosia/keys"
	"github.com/steven3002/mnemosia/local"
	"github.com/steven3002/mnemosia/manifest"
	"github.com/steven3002/mnemosia/recall"
	"github.com/steven3002/mnemosia/seal"
	"github.com/steven3002/mnemosia/sia"
	"github.com/steven3002/mnemosia/store"
	"github.com/steven3002/mnemosia/store/packer"
	"github.com/steven3002/mnemosia/store/reclaim"
)

// ErrWrongPhrase reports that the recovery phrase does not match the one this
// vault was created with.
var ErrWrongPhrase = errors.New("recovery phrase does not match this vault")

// A Vault is the public API. It orchestrates every operation and owns nothing
// that another package could own instead.
type Vault struct {
	opts      Options
	sealer    *seal.Sealer
	local     *local.Store
	manifest  *manifest.Manifest
	index     *index.Index
	embedder  *embed.Embedder
	client    *sia.Client
	store     *store.Store
	packer    *packer.Packer
	reclaimer *reclaim.Reclaimer
	vectors   *index.Store
	health    IndexHealth
}

// Open prepares a vault for use.
//
// The recovery phrase is consumed here and not retained: every key the process
// needs is derived up front, so the phrase itself has no reason to survive the
// call.
func Open(ctx context.Context, opts Options) (*Vault, error) {
	opts.applyDefaults()
	if err := os.MkdirAll(opts.Home, 0o700); err != nil {
		return nil, fmt.Errorf("prepare vault directory %s: %w", opts.Home, err)
	}

	seed, err := keys.SeedFromPhrase(opts.Phrase)
	if err != nil {
		return nil, err
	}
	hierarchy, err := keys.Derive(seed)
	seed.Wipe()
	if err != nil {
		return nil, err
	}
	defer hierarchy.Wipe()

	v := &Vault{opts: opts}
	if err := v.openLocal(hierarchy); err != nil {
		v.Close()
		return nil, err
	}
	if err := v.openIndex(ctx); err != nil {
		v.Close()
		return nil, err
	}
	if !opts.Offline {
		if err := v.connect(ctx); err != nil {
			v.Close()
			return nil, err
		}
	}
	// The packer exists whether or not there is a connection. Offline it can
	// only queue, but queueing is the part that must not be skipped: a record
	// written with no connection is owed to the network exactly as much as one
	// written with a connection, and the earlier design silently owed nothing.
	if err := v.openPacker(); err != nil {
		v.Close()
		return nil, err
	}
	return v, nil
}

func (v *Vault) openLocal(hierarchy keys.Hierarchy) error {
	sealer, err := seal.New(hierarchy.Record, hierarchy.Content)
	if err != nil {
		return err
	}
	v.sealer = sealer

	store, err := local.Open(v.opts.dbPath())
	if err != nil {
		return err
	}
	v.local = store

	if err := v.checkPhrase(hierarchy); err != nil {
		return err
	}

	manifestSealer, err := seal.New(hierarchy.Manifest, hierarchy.Content)
	if err != nil {
		return err
	}
	log, err := manifest.OpenLog(v.opts.manifestDir(), manifestSealer)
	if err != nil {
		return err
	}
	catalog, err := manifest.Load(log)
	if err != nil {
		return err
	}
	v.manifest = catalog
	return nil
}

// checkPhrase stores a derived check value on first open and compares it
// afterwards, so a mistyped phrase reports itself instead of quietly opening a
// second, empty vault over the first one's files.
func (v *Vault) checkPhrase(hierarchy keys.Hierarchy) error {
	const key = "phraseCheck"
	want := fmt.Sprintf("%x", hierarchy.Check)

	got, err := v.local.GetMeta(key)
	if errors.Is(err, local.ErrNotFound) {
		return v.local.PutMeta(key, want)
	}
	if err != nil {
		return err
	}
	if got != want {
		return ErrWrongPhrase
	}
	return nil
}

func (v *Vault) openIndex(ctx context.Context) error {
	dir, err := v.opts.Model.Fetch(ctx, v.opts.ModelDir)
	if err != nil {
		return err
	}
	embedder, err := embed.Open(ctx, v.opts.Model, dir)
	if err != nil {
		return err
	}
	v.embedder = embedder
	v.index = index.New(v.opts.Model.Name, embedder.Dim())

	vectors, err := index.OpenStore(v.opts.indexDir(), v.sealer)
	if err != nil {
		return err
	}
	v.vectors = vectors

	stored, err := vectors.Hydrate()
	if err != nil {
		return err
	}
	usable, health := classifyVectors(v.opts.Model.Name, embedder.Dim(), stored)
	for _, vector := range usable {
		if err := v.index.Add(vector.ID, vector.Model, vector.Vector); err != nil {
			return err
		}
	}
	v.health = health
	return nil
}

// classifyVectors separates the vectors this index can search from the ones it
// cannot, and counts what it left behind.
//
// A vector from another model belongs to a different space, so scoring it
// against this query would be meaningless rather than merely worse. It is
// counted rather than passed over in silence: a vault that quietly searches a
// tenth of itself looks exactly like a vault with poor recall, and the two want
// completely different responses.
func classifyVectors(model string, dim int, stored []index.Entry) ([]index.Entry, IndexHealth) {
	health := IndexHealth{Model: model, Dim: dim, Foreign: map[string]int{}}
	usable := make([]index.Entry, 0, len(stored))
	for _, vector := range stored {
		if vector.Model != model || vector.Dim != dim {
			health.Foreign[vector.Model]++
			continue
		}
		usable = append(usable, vector)
		health.Indexed++
	}
	return usable, health
}

// An IndexHealth reports what the index was built from.
//
// It exists because the interesting failure here is silent. Vectors from two
// models cannot be compared, but comparing them produces a number rather than an
// error, so an index that has been half re-embedded ranks confidently and
// wrongly. Counting what was left out is what turns that into something a user
// can be told.
type IndexHealth struct {
	// Model and Dim are what this index accepts.
	Model string
	Dim   int
	// Indexed is how many vectors are searchable.
	Indexed int
	// Foreign counts stored vectors held under some other model, by model name.
	Foreign map[string]int
}

// Mixed reports whether the device holds vectors this index cannot search.
func (h IndexHealth) Mixed() bool { return len(h.Foreign) > 0 }

// Stale reports how many records are on the device but absent from the index.
func (h IndexHealth) Stale() int {
	var n int
	for _, count := range h.Foreign {
		n += count
	}
	return n
}

// IndexHealth reports what the search index was built from, including any
// vectors it had to leave out.
func (v *Vault) IndexHealth() IndexHealth { return v.health }

func (v *Vault) connect(ctx context.Context) error {
	client, err := sia.Connect(sia.Config{Indexer: v.opts.Indexer, AppKey: v.opts.AppKey})
	if err != nil {
		return err
	}
	v.client = client
	v.store = store.New(client)
	v.reclaimer = reclaim.New(client, v.local)
	_ = ctx
	return nil
}

// openPacker prepares the write queue.
//
// The flush policy needs the slab size, which only the network knows, so an
// offline vault falls back to the measured default. It affects nothing that
// matters offline: the byte trigger is the one deadline that never fires in
// practice, and a flush cannot happen without a connection anyway.
func (v *Vault) openPacker() error {
	policy := v.opts.Flush
	if policy == (packer.Policy{}) {
		slabBytes := int64(store.DefaultSlabPayloadSize)
		if v.store != nil {
			measured, err := v.store.SlabPayloadSize()
			if err != nil {
				return err
			}
			slabBytes = measured
		}
		policy = packer.DefaultPolicy(slabBytes)
	}

	queue, err := packer.New(v.store, v.local, policy)
	if err != nil {
		return err
	}
	v.packer = queue
	v.packer.OnFlush(v.recordFlush)
	return nil
}

// StartFlushing runs the flush deadlines in the background until the vault is
// closed.
//
// It is opt-in because a short-lived command has nothing to gain from a timer:
// it flushes explicitly, or leaves the queue for the next run. A long-running
// host is the case the deadlines were written for.
func (v *Vault) StartFlushing(ctx context.Context, onError func(error)) {
	if v.packer != nil {
		v.packer.Start(ctx, onError)
	}
}

// Close releases everything the vault holds. Records still queued for a flush
// stay on this device and are written by a later run.
func (v *Vault) Close() error {
	var errs []error
	if v.packer != nil {
		v.packer.Stop()
	}
	if v.manifest != nil {
		errs = append(errs, v.manifest.Close())
	}
	if v.vectors != nil {
		errs = append(errs, v.vectors.Close())
	}
	if v.embedder != nil {
		errs = append(errs, v.embedder.Close())
	}
	if v.client != nil {
		errs = append(errs, v.client.Close())
	}
	if v.local != nil {
		errs = append(errs, v.local.Close())
	}
	if v.sealer != nil {
		v.sealer.Close()
	}
	return errors.Join(errs...)
}

// Online reports whether the vault has a connection to an indexer.
func (v *Vault) Online() bool { return v.client != nil }

// Indexer reports which indexer the vault is connected to.
func (v *Vault) Indexer() string {
	if v.client == nil {
		return ""
	}
	return v.client.Indexer()
}

// Account reports the vault's standing with the indexer.
func (v *Vault) Account(ctx context.Context) (sia.Account, error) {
	if v.client == nil {
		return sia.Account{}, errOffline
	}
	return v.client.Account(ctx)
}

// WaitReady blocks until the indexer can accept a write.
func (v *Vault) WaitReady(ctx context.Context, budget time.Duration) (sia.Account, error) {
	if v.client == nil {
		return sia.Account{}, errOffline
	}
	return v.client.WaitReady(ctx, budget)
}

// Pending reports how many records are held on this device but not yet written
// to the network.
func (v *Vault) Pending() int {
	if v.packer == nil {
		return 0
	}
	return v.packer.Pending()
}

// PendingBytes reports the sealed size of what is queued, counted as it will be
// written: ciphertext with its envelope and framing, which is what a slab is
// actually filled with.
func (v *Vault) PendingBytes() int64 {
	if v.packer == nil {
		return 0
	}
	return v.packer.PendingBytes()
}

// Recall retrieves records by meaning.
func (v *Vault) Recall(ctx context.Context, req recall.Request) (recall.Result, error) {
	return recall.New(v.embedder, v.index, v, v).Run(ctx, req)
}

var errOffline = errors.New("vault is open offline")
