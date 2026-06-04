package retriever

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockEmbedder struct {
	mock.Mock
}

func (m *mockEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	args := m.Called(ctx, chunks)
	return args.Get(0).([]types.Embedding), args.Error(1)
}

func (m *mockEmbedder) Dimensions() int {
	args := m.Called()
	return args.Int(0)
}

func (m *mockEmbedder) ModelName() string {
	args := m.Called()
	return args.String(0)
}

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

func (m *mockStore) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNew(t *testing.T) {
	e := new(mockEmbedder)
	s := new(mockStore)
	r := New(e, s)
	assert.NotNil(t, r)
	assert.Equal(t, e, r.embedder)
	assert.Equal(t, s, r.store)
}

func TestRetrieve(t *testing.T) {
	e := new(mockEmbedder)
	s := new(mockStore)
	r := New(e, s)

	ctx := context.Background()
	expectedResults := []types.SearchResult{
		{ChunkID: "chunk1", DocumentPath: "doc1.md", Content: "content1", Score: 0.95},
		{ChunkID: "chunk2", DocumentPath: "doc2.md", Content: "content2", Score: 0.85},
	}

	e.On("Embed", ctx, []types.Chunk{{ID: "query", Content: "test query"}}).
		Return([]types.Embedding{{Vector: []float64{0.1, 0.2, 0.3}}}, nil)

	s.On("Search", ctx, "my-collection", []float32{0.1, 0.2, 0.3}, 5).
		Return(expectedResults, nil)

	results, err := r.Retrieve(ctx, "my-collection", "test query", 5)
	assert.NoError(t, err)
	assert.Equal(t, expectedResults, results)
	e.AssertExpectations(t)
	s.AssertExpectations(t)
}

func TestRetrieve_EmbedError(t *testing.T) {
	e := new(mockEmbedder)
	s := new(mockStore)
	r := New(e, s)

	ctx := context.Background()
	e.On("Embed", ctx, []types.Chunk{{ID: "query", Content: "query"}}).
		Return([]types.Embedding{}, errors.New("api error"))

	_, err := r.Retrieve(ctx, "col", "query", 5)
	assert.ErrorContains(t, err, "embed query")
}

func TestRetrieve_EmptyEmbeddings(t *testing.T) {
	e := new(mockEmbedder)
	s := new(mockStore)
	r := New(e, s)

	ctx := context.Background()
	e.On("Embed", ctx, []types.Chunk{{ID: "query", Content: "query"}}).
		Return([]types.Embedding{}, nil)

	_, err := r.Retrieve(ctx, "col", "query", 5)
	assert.ErrorContains(t, err, "no embeddings returned")
}

func TestRetrieve_SearchError(t *testing.T) {
	e := new(mockEmbedder)
	s := new(mockStore)
	r := New(e, s)

	ctx := context.Background()
	e.On("Embed", ctx, []types.Chunk{{ID: "query", Content: "query"}}).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	s.On("Search", ctx, "col", []float32{0.1}, 5).
		Return([]types.SearchResult{}, errors.New("search failed"))

	_, err := r.Retrieve(ctx, "col", "query", 5)
	assert.ErrorContains(t, err, "search")
}
