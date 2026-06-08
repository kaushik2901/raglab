package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockEmbedder struct {
	embedFn func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error)
}

func (m *mockEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	return m.embedFn(ctx, chunks)
}

func (m *mockEmbedder) Dimensions() int  { return 0 }
func (m *mockEmbedder) ModelName() string { return "mock" }

type mockStore struct {
	storeFn func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error
}

func (m *mockStore) Connect(ctx context.Context, dsn string) error { return nil }
func (m *mockStore) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	return nil
}
func (m *mockStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
	return m.storeFn(ctx, collectionName, chunks)
}
func (m *mockStore) Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	return nil, nil
}
func (m *mockStore) Close() error { return nil }

func TestEmbedAndStore_ContextCancel(t *testing.T) {
	emb := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	st := &mockStore{
		storeFn: func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := embedAndStore(ctx, emb, st, "test", []types.Chunk{{ID: "c1"}})
	assert.Error(t, err)
}

func TestEmbedAndStore_ParentTimeout(t *testing.T) {
	emb := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	st := &mockStore{
		storeFn: func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
			return nil
		},
	}

	// embedAndStore creates batchCtx from parent; parent expiry propagates
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(5 * time.Millisecond)

	err := embedAndStore(ctx, emb, st, "test", []types.Chunk{{ID: "c1"}})
	assert.Error(t, err)
}

func TestEmbedAndStore_Success(t *testing.T) {
	emb := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			result := make([]types.Embedding, len(chunks))
			for i := range chunks {
				result[i] = types.Embedding{
					ChunkID:    chunks[i].ID,
					Vector:     []float64{0.1, 0.2},
					Dimensions: 2,
				}
			}
			return result, nil
		},
	}
	st := &mockStore{
		storeFn: func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
			return nil
		},
	}

	chunks := []types.Chunk{
		{ID: "c1", Content: "hello"},
		{ID: "c2", Content: "world"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := embedAndStore(ctx, emb, st, "test", chunks)
	assert.NoError(t, err)
}
