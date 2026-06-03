package embedder

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type Embedder interface {
	Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error)
	Dimensions() int
	ModelName() string
}
