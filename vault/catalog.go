package vault

import (
	"github.com/steven3002/mnemosia/local"
	"github.com/steven3002/mnemosia/recall"
	"github.com/steven3002/mnemosia/record"
)

// Describe answers the ranking pipeline's questions about a candidate pool.
//
// The vault is the adapter here rather than the store implementing the recall
// interface directly, so that `local` stays a storage package that knows nothing
// about ranking, and `recall` keeps depending on nothing but the index, the
// embedder and the record schema.
func (v *Vault) Describe(ids []record.ID) (recall.Described, error) {
	described, err := v.local.Describe(ids)
	if err != nil {
		return recall.Described{}, err
	}
	out := recall.Described{
		Meta:       make(map[record.ID]recall.Meta, len(described.Meta)),
		Superseded: described.Superseded,
	}
	for id, meta := range described.Meta {
		out.Meta[id] = recall.Meta{Type: meta.Type, Tags: meta.Tags}
	}
	return out, nil
}

// rankingMeta is what a memory contributes to the ranking metadata tables.
func rankingMeta(memory *record.Memory) local.RankingMeta {
	return local.RankingMeta{
		ID:         memory.ID,
		Type:       memory.Type,
		Tags:       memory.Tags,
		Supersedes: memory.Supersedes,
	}
}
