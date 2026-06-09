package store

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type VectorStore interface {
	Connect(ctx context.Context, dsn string) error

	EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error

	Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error

	Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error)

	HealthCheck(ctx context.Context) error

	Close() error
}
