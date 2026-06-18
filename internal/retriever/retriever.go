package retriever

import (
	"context"
	"fmt"
	"math"

	qstore "github.com/kaushik2901/raglab/internal/store"
	"github.com/kaushik2901/raglab/internal/types"
)

const (
	StrategyNaiveSearch = "naive-search"
	StrategyMMR         = "mmr-rerank"

	mmrDefaultLambda   = 0.7
	mmrFetchMultiplier = 3
)

type RetrieveFunc func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error)

var strategies = map[string]func(qstore.VectorStore) RetrieveFunc{
	StrategyNaiveSearch: naiveSearchFactory,
	StrategyMMR:         mmrSearchFactory,
}

type Retriever struct {
	store    qstore.VectorStore
	strategy string
}

func New(store qstore.VectorStore, strategy string) (*Retriever, error) {
	if _, ok := strategies[strategy]; !ok {
		return nil, fmt.Errorf("unknown retrieval strategy: %q (supported: %s, %s)", strategy, StrategyNaiveSearch, StrategyMMR)
	}
	return &Retriever{store: store, strategy: strategy}, nil
}

func (r *Retriever) Retrieve(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	fn, ok := strategies[r.strategy]
	if !ok {
		return nil, fmt.Errorf("strategy %q not implemented", r.strategy)
	}
	return fn(r.store)(ctx, collection, queryVector, topK)
}

func RegisterRetriever(name string, fn func(qstore.VectorStore) RetrieveFunc) {
	strategies[name] = fn
}

func naiveSearchFactory(store qstore.VectorStore) RetrieveFunc {
	return func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
		return store.Search(ctx, collection, queryVector, topK)
	}
}

func mmrSearchFactory(store qstore.VectorStore) RetrieveFunc {
	return func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
		fetchK := topK * mmrFetchMultiplier
		results, err := store.Search(ctx, collection, queryVector, fetchK)
		if err != nil {
			return nil, fmt.Errorf("mmr fetch: %w", err)
		}
		return RerankMMR(results, queryVector, mmrDefaultLambda, topK), nil
	}
}

func RerankMMR(results []types.SearchResult, queryVector []float32, lambda float64, topK int) []types.SearchResult {
	if len(results) <= topK || topK <= 0 {
		return results
	}

	n := len(results)
	sim := make([][]float64, n)
	for i := range sim {
		sim[i] = make([]float64, n)
		for j := range sim[i] {
			sim[i][j] = cosineSimilarity(results[i].Vector, results[j].Vector)
		}
	}

	selected := make([]int, 0, topK)
	remaining := make([]int, n)
	for i := range remaining {
		remaining[i] = i
	}

	for len(selected) < topK {
		bestScore := -1.0
		bestIdx := -1
		bestPos := -1

		for pos, idx := range remaining {
			var maxPairSim float64
			for _, s := range selected {
				if sim[idx][s] > maxPairSim {
					maxPairSim = sim[idx][s]
				}
			}
			mmr := lambda*float64(results[idx].Score) - (1-lambda)*maxPairSim
			if mmr > bestScore {
				bestScore = mmr
				bestIdx = idx
				bestPos = pos
			}
		}

		selected = append(selected, bestIdx)
		remaining = append(remaining[:bestPos], remaining[bestPos+1:]...)
	}

	reranked := make([]types.SearchResult, topK)
	for i, idx := range selected {
		reranked[i] = results[idx]
	}
	return reranked
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
