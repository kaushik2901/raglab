package retriever

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) Connect(ctx context.Context, dsn string) error {
	args := m.Called(ctx, dsn)
	return args.Error(0)
}

func (m *mockStore) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	args := m.Called(ctx, name, vectorSize, distance)
	return args.Error(0)
}

func (m *mockStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
	args := m.Called(ctx, collectionName, chunks)
	return args.Error(0)
}

func (m *mockStore) Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	args := m.Called(ctx, collectionName, queryVector, topK)
	return args.Get(0).([]types.SearchResult), args.Error(1)
}

func (m *mockStore) ListCollections(ctx context.Context) ([]store.CollectionInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).([]store.CollectionInfo), args.Error(1)
}

func (m *mockStore) GetCollection(ctx context.Context, name string) (*store.CollectionInfo, error) {
	args := m.Called(ctx, name)
	return args.Get(0).(*store.CollectionInfo), args.Error(1)
}

func (m *mockStore) DeleteCollection(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

func (m *mockStore) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockStore) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNew(t *testing.T) {
	s := new(mockStore)
	r, err := New(s, StrategyNaiveSearch)
	assert.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, StrategyNaiveSearch, r.strategy)
}

func TestNew_UnknownStrategy(t *testing.T) {
	s := new(mockStore)
	_, err := New(s, "hybrid-search")
	assert.ErrorContains(t, err, "unknown retrieval strategy")
}

func TestRetrieve_NaiveSearch(t *testing.T) {
	s := new(mockStore)
	r, err := New(s, StrategyNaiveSearch)
	require.NoError(t, err)

	ctx := context.Background()
	queryVector := []float32{0.1, 0.2, 0.3}
	expectedResults := []types.SearchResult{
		{ChunkID: "chunk1", DocumentPath: "doc1.md", Content: "content1", Score: 0.95},
		{ChunkID: "chunk2", DocumentPath: "doc2.md", Content: "content2", Score: 0.85},
	}

	s.On("Search", ctx, "my-collection", queryVector, 5).
		Return(expectedResults, nil)

	results, err := r.Retrieve(ctx, "my-collection", queryVector, 5)
	assert.NoError(t, err)
	assert.Equal(t, expectedResults, results)
	s.AssertExpectations(t)
}

func TestRetrieve_MMR_ReranksToTopK(t *testing.T) {
	s := new(mockStore)
	r, err := New(s, StrategyMMR)
	require.NoError(t, err)

	ctx := context.Background()
	queryVector := []float32{1, 0}

	// 5 candidates but fetchK=topK*3 means fetchK=15, but we only provide 5
	results := []types.SearchResult{
		{DocumentPath: "a.md", Score: 0.9, Content: "a", Vector: []float32{1, 0}},
		{DocumentPath: "b.md", Score: 0.8, Content: "b", Vector: []float32{0.9, 0.1}},
		{DocumentPath: "c.md", Score: 0.7, Content: "c", Vector: []float32{0, 1}},
	}

	s.On("Search", ctx, "col", queryVector, 3*mmrFetchMultiplier).Return(results, nil)

	reranked, err := r.Retrieve(ctx, "col", queryVector, 3)
	assert.NoError(t, err)
	assert.Len(t, reranked, 3)
	// Result order may differ from input due to MMR, but all 3 should be present
	paths := make(map[string]bool)
	for _, r := range reranked {
		paths[r.DocumentPath] = true
	}
	assert.True(t, paths["a.md"])
	assert.True(t, paths["b.md"])
	assert.True(t, paths["c.md"])
}

func TestRerankMMR_EmptyInput(t *testing.T) {
	assert.Nil(t, RerankMMR(nil, []float32{0.1}, 0.7, 5))
	assert.Empty(t, RerankMMR([]types.SearchResult{}, []float32{0.1}, 0.7, 5))
}

func TestRerankMMR_LessThanTopK(t *testing.T) {
	results := []types.SearchResult{
		{DocumentPath: "a.md", Score: 0.9, Vector: []float32{1, 0}},
	}
	out := RerankMMR(results, []float32{1, 0}, 0.7, 5)
	assert.Len(t, out, 1)
	assert.Equal(t, "a.md", out[0].DocumentPath)
}
