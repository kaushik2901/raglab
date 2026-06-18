package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/raglab/internal/config"
	qstore "github.com/kaushik2901/raglab/internal/store"
	"github.com/kaushik2901/raglab/internal/types"
)

type mockVectorStore struct{}

func (m *mockVectorStore) Connect(ctx context.Context, dsn string) error { return nil }
func (m *mockVectorStore) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	return nil
}
func (m *mockVectorStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
	return nil
}
func (m *mockVectorStore) Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	return nil, nil
}
func (m *mockVectorStore) ListCollections(ctx context.Context) ([]qstore.CollectionInfo, error) { return nil, nil }
func (m *mockVectorStore) GetCollection(ctx context.Context, name string) (*qstore.CollectionInfo, error) {
	return nil, nil
}
func (m *mockVectorStore) DeleteCollection(ctx context.Context, name string) error { return nil }
func (m *mockVectorStore) HealthCheck(ctx context.Context) error                   { return nil }
func (m *mockVectorStore) Close() error                                            { return nil }

func TestNewWithDeps_ServerInitialized(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		MaxRetries:        3,
		RetryBackoff:      5 * time.Second,
		LLMBaseURL:        "https://api.openai.com",
		APIRequestTimeout: 60 * time.Second,
		ChatMemorySize:    10,
	}

	s := NewWithDeps(cfg, nil, &mockVectorStore{})
	require.NotNil(t, s)
	assert.NotNil(t, s.router)
	assert.Equal(t, cfg, s.cfg)
	assert.Nil(t, s.pool)
}

func TestNewWithDeps_WithVectorStore(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		MaxRetries:        3,
		RetryBackoff:      5 * time.Second,
		LLMBaseURL:        "https://api.openai.com",
		APIRequestTimeout: 60 * time.Second,
		ChatMemorySize:    10,
	}

	vs := &mockVectorStore{}
	s := NewWithDeps(cfg, nil, vs)
	require.NotNil(t, s)
	assert.Equal(t, vs, s.qdrant)
}
