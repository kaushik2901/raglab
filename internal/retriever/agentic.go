package retriever

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func init() {
	RegisterRetriever("agentic", func(r *Retriever) RetrievalFunc {
		return r.agenticSearch
	})
}

func (r *Retriever) agenticSearch(ctx context.Context, coll, query string, topK int) ([]types.SearchResult, error) {
	return r.naiveSearch(ctx, coll, query, topK)
}
