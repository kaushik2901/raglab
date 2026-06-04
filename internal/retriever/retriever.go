package retriever

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

const StrategyNaiveSearch = "naive-search"

var validStrategies = map[string]bool{
	StrategyNaiveSearch: true,
}

type Retriever struct {
	embedder embedder.Embedder
	store    qstore.VectorStore
	strategy string
}

func New(embedder embedder.Embedder, store qstore.VectorStore, strategy string) (*Retriever, error) {
	if !validStrategies[strategy] {
		return nil, fmt.Errorf("unknown query strategy: %q (supported: naive-search)", strategy)
	}
	return &Retriever{embedder: embedder, store: store, strategy: strategy}, nil
}

func (r *Retriever) Retrieve(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error) {
	switch r.strategy {
	case StrategyNaiveSearch:
		return r.naiveSearch(ctx, collection, query, topK)
	default:
		return nil, fmt.Errorf("strategy %q not implemented", r.strategy)
	}
}

func (r *Retriever) naiveSearch(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error) {
	queryChunk := types.Chunk{ID: "query", Content: query}
	embeddings, err := r.embedder.Embed(ctx, []types.Chunk{queryChunk})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	queryVector := make([]float32, len(embeddings[0].Vector))
	for i, v := range embeddings[0].Vector {
		queryVector[i] = float32(v)
	}

	results, err := r.store.Search(ctx, collection, queryVector, topK)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return results, nil
}
