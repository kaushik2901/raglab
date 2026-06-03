package store

import (
	"context"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type VectorStore interface {
	Connect(ctx context.Context, dsn string) error

	EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error

	Store(ctx context.Context, chunks []types.DocumentChunk) error

	Close() error
}
