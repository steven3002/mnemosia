package embed

import "context"

// A Vectorizer turns text into vectors and says which model produced them.
//
// It exists so that the layers above can be built and exercised without a
// model resident in the process. The model is a third of a gigabyte once
// loaded, which is affordable once and not affordable in every test binary at
// the same time; and most of what the vault does with a vector — storing it,
// persisting it, ranking on it, superseding a record that has one — is
// indifferent to which model produced it.
//
// Model is part of the interface rather than something the caller configures
// separately, because a vector and the model that made it are a pair. Two
// models embed into different spaces, and comparing across them yields a number
// with no meaning that ranks perfectly happily.
type Vectorizer interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedOne(ctx context.Context, text string) ([]float32, error)
	Model() Model
	Dim() int
	Close() error
}

// Embedder is the model-backed implementation.
var _ Vectorizer = (*Embedder)(nil)
