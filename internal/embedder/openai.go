package embedder

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type embedder struct {
	model      string
	batchSize  int
	client     openai.Client
	dimensions atomic.Int32
}

func newOpenAIEmbedder(baseURL, apiKey, model string, batchSize int) *embedder {
	config.WarnOnInsecure(baseURL, apiKey, "embedder")
	opts := []option.RequestOption{
		option.WithBaseURL(config.NormalizeBaseURL(baseURL) + "/v1/"),
		option.WithHeader("Content-Type", "application/json"),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	opts = append(opts, option.WithMaxRetries(0))
	return &embedder{
		client:    openai.NewClient(opts...),
		model:     model,
		batchSize: batchSize,
	}
}

func (e *embedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	var embeddings []types.Embedding
	for i := 0; i < len(chunks); i += e.batchSize {
		end := min(i+e.batchSize, len(chunks))
		batch := chunks[i:end]

		batchEmbeddings, err := e.embedBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embed batch %d-%d: %w", i, end, err)
		}
		embeddings = append(embeddings, batchEmbeddings...)
	}

	return embeddings, nil
}

func (e *embedder) embedBatch(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	inputs := make([]string, len(chunks))
	for i, ch := range chunks {
		inputs[i] = ch.Content
	}

	params := openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(e.model),
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: inputs,
		},
	}

	resp, err := e.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}

	if len(resp.Data) != len(chunks) {
		return nil, fmt.Errorf("response has %d embeddings, expected %d", len(resp.Data), len(chunks))
	}

	modelName := resp.Model
	if modelName == "" {
		modelName = e.model
	}

	embeddings := make([]types.Embedding, len(chunks))
	for i, d := range resp.Data {
		e.dimensions.CompareAndSwap(0, int32(len(d.Embedding)))
		embeddings[i] = types.Embedding{
			ChunkID:    chunks[i].ID,
			Vector:     d.Embedding,
			Model:      modelName,
			Dimensions: len(d.Embedding),
		}
	}

	return embeddings, nil
}

func (e *embedder) Dimensions() int {
	return int(e.dimensions.Load())
}

func (e *embedder) ModelName() string {
	return e.model
}
