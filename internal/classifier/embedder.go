package classifier

import (
	"context"
	"fmt"
	minilm "github.com/clems4ever/all-minilm-l6-v2-go/all_minilm_l6_v2"
)

// Embedder converts text into a 384-dimensional vector.
// All three classifiers depend on this interface.
// The real implementation uses MiniLM. Tests use a deterministic mock.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Close()
}

// MiniLMEmbedder is the production Embedder backed by all-MiniLM-L6-v2 via ONNX.
// One instance loads at startup and serves all requests for the server lifetime.
type MiniLMEmbedder struct {
	model *minilm.Model
}
// NewEmbedder loads the MiniLM model from the ONNX runtime.
// runtimePath is the path to libonnxruntime.so on the host or in the container.
// Fatal at startup if the model cannot load — the server cannot function without it.
func NewEmbedder(runtimePath string) (*MiniLMEmbedder, error) {
	model, err := minilm.NewModel(minilm.WithRuntimePath(runtimePath))
	if err != nil{
		return nil, fmt.Errorf("failed to load embedding model: %w", err)
	}
	return &MiniLMEmbedder{model: model}, nil
}

func (e *MiniLMEmbedder) Embed(_ context.Context, text string) ([]float32, error){
	vec, err := e.model.Compute(text)	
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	return vec, nil
}

func (e *MiniLMEmbedder) EmbedBatch(_ context.Context, text []string) ([][]float32, error) {
	vecs, err := e.model.ComputeBatch(text)
	if err != nil {
		return nil, fmt.Errorf("embed batch: %w", err)
	}
	return vecs, nil
}

func (e *MiniLMEmbedder) Close() {
	e.model.Close()
}

// cosineSimilarity returns the similarity between two unit-normalised vectors.
// MiniLM produces unit-normalised output so this reduces to a dot product.
func cosineSimilarity(a, b []float32) float32 {
	return minilm.CosineSimilarity(a, b)
}