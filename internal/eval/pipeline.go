package eval

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

const SystemPrompt = "You are a helpful assistant that answers questions based solely on the provided context. If the context does not contain enough information to answer, say so."

type VectorSearcher interface {
	Search(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error)
}
