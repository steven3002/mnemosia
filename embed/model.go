// Package embed converts text to a vector.
package embed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/knights-analytics/hugot"
)

// Model describes an embedding model and where its files live.
type Model struct {
	// Name is stored beside every vector, so a model change is a detectable
	// re-index rather than a silent corruption of the index.
	Name string
	// Repo is the model's upstream location.
	Repo string
	// OnnxFile is the file to fetch from the repo.
	OnnxFile string
	// Dim is the vector width.
	Dim int
}

// BGESmallEN is the default: 384 dimensions in full precision.
//
// The quantized build of the same model is deliberately not offered on the
// pure-Go backend. It is strictly worse there, several times slower for
// identical ranking, because that backend has no optimized kernels for it, so
// making it reachable would only be a way to choose the worse option.
var BGESmallEN = Model{
	Name:     "bge-small-en-v1.5-fp32",
	Repo:     "Xenova/bge-small-en-v1.5",
	OnnxFile: "onnx/model.onnx",
	Dim:      384,
}

// Dir returns the directory a model's files occupy under root.
func (m Model) Dir(root string) string {
	return filepath.Join(root, m.Name, repoDirName(m.Repo))
}

// onnxBasename is the filename inside the model directory. The downloader
// flattens the repo's subdirectory away, so the path in the repo and the path
// on disk differ.
func (m Model) onnxBasename() string {
	if m.OnnxFile == "" {
		return "model.onnx"
	}
	return filepath.Base(m.OnnxFile)
}

// Present reports whether the model is already on disk under root.
func (m Model) Present(root string) bool {
	_, err := os.Stat(filepath.Join(m.Dir(root), m.onnxBasename()))
	return err == nil
}

// Fetch downloads the model under root unless it is already there.
func (m Model) Fetch(ctx context.Context, root string) (string, error) {
	dir := m.Dir(root)
	if m.Present(root) {
		return dir, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("prepare model directory %s: %w", root, err)
	}
	opts := hugot.NewDownloadOptions()
	opts.OnnxFilePath = m.OnnxFile
	opts.Verbose = false

	ctx, cancel := context.WithTimeout(ctx, downloadBudget)
	defer cancel()
	if _, err := hugot.DownloadModel(ctx, m.Repo, filepath.Join(root, m.Name), opts); err != nil {
		return "", fmt.Errorf("download %s: %w", m.Repo, err)
	}
	if !m.Present(root) {
		return "", fmt.Errorf("download %s: %s is missing afterwards", m.Repo, m.onnxBasename())
	}
	return dir, nil
}

const downloadBudget = 30 * time.Minute

func repoDirName(repo string) string {
	out := []rune(repo)
	for i, r := range out {
		if r == '/' {
			out[i] = '_'
		}
	}
	return string(out)
}
