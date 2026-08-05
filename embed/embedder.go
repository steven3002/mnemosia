package embed

import (
	"context"
	"fmt"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/backends"
	"github.com/knights-analytics/hugot/pipelines"
)

// An Embedder turns text into vectors.
//
// The query is embedded on the device like everything else, so a search never
// discloses what was searched for. That is only true because there is no
// hosted model in the path.
type Embedder struct {
	model   Model
	session *hugot.Session
	pipe    *pipelines.FeatureExtractionPipeline
}

// Open loads a model from dir on the backend this binary was built with.
func Open(ctx context.Context, model Model, dir string) (*Embedder, error) {
	session, err := newSession(ctx)
	if err != nil {
		return nil, err
	}
	pipe, err := hugot.NewPipeline(session, hugot.FeatureExtractionConfig{
		ModelPath:    dir,
		Name:         model.Name,
		OnnxFilename: model.onnxBasename(),
		// Vectors come back unit length, which makes cosine similarity a plain
		// dot product for every comparison the index will ever run.
		Options: []backends.PipelineOption[*pipelines.FeatureExtractionPipeline]{
			pipelines.WithNormalization(),
		},
	})
	if err != nil {
		session.Destroy()
		return nil, fmt.Errorf("load %s from %s: %w", model.Name, dir, err)
	}
	return &Embedder{model: model, session: session, pipe: pipe}, nil
}

// Close releases the model.
func (e *Embedder) Close() error { return e.session.Destroy() }

// Model reports which model is loaded.
func (e *Embedder) Model() Model { return e.model }

// Dim reports the vector width.
func (e *Embedder) Dim() int { return e.model.Dim }

// Backend names the runtime this binary was built against.
func (e *Embedder) Backend() string { return backendName }

// Embed runs a batch of texts through the model.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out, err := e.pipe.RunPipeline(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed %d text(s): %w", len(texts), err)
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embed returned %d vectors for %d texts", len(out.Embeddings), len(texts))
	}
	for i, vector := range out.Embeddings {
		if len(vector) != e.model.Dim {
			return nil, fmt.Errorf("embed returned %d dimensions for text %d, model %s declares %d",
				len(vector), i, e.model.Name, e.model.Dim)
		}
	}
	return out.Embeddings, nil
}

// EmbedOne runs a single text through the model.
func (e *Embedder) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}
