package embedder

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type Embedder interface {
	Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error)
	Dimensions() int
	ModelName() string
}

// New creates an Embedder for the given provider and model.
// It resolves the provider-specific base URL and API key from environment variables.
func New(provider config.Provider, model string, batchSize int) (Embedder, error) {
	baseURL, apiKey := config.ResolveProviderConfig(provider)
	if baseURL == "" {
		return nil, fmt.Errorf("empty base URL for provider %q", provider)
	}
	var e Embedder = newOpenAIEmbedder(baseURL, apiKey, model, batchSize)
	rpm := config.FloatEnvOrDefault("EMBEDDER_RATE_LIMIT_RPM", 100)
	if rpm > 0 {
		e = NewRateLimitedEmbedder(e, rpm)
	}
	return e, nil
}
