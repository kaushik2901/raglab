package store

import (
	"context"
	"fmt"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type CollectionInfo struct {
	Name        string `json:"name"`
	VectorCount uint64 `json:"vector_count"`
	VectorSize  uint64 `json:"vector_size"`
	Distance    string `json:"distance"`
}

var ErrCollectionNotFound = fmt.Errorf("collection not found")

type VectorStore interface {
	Connect(ctx context.Context, dsn string) error

	EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error

	Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error

	Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error)

	ListCollections(ctx context.Context) ([]CollectionInfo, error)
	GetCollection(ctx context.Context, name string) (*CollectionInfo, error)
	DeleteCollection(ctx context.Context, name string) error

	HealthCheck(ctx context.Context) error

	Close() error
}
