package retriever

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func init() {
	RegisterRetriever("hybrid", func(r *Retriever) RetrievalFunc {
		return r.hybridSearch
	})
}

func (r *Retriever) hybridSearch(ctx context.Context, coll, query string, topK int) ([]types.SearchResult, error) {
	return r.naiveSearch(ctx, coll, query, topK)
}
