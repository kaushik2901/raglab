package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sony/gobreaker"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type CircuitBreakerVectorStore struct {
	inner         VectorStore
	storeBreaker  *gobreaker.CircuitBreaker
	searchBreaker *gobreaker.CircuitBreaker
	ensureBreaker *gobreaker.CircuitBreaker
}

func NewCircuitBreakerVectorStore(inner VectorStore) *CircuitBreakerVectorStore {
	breaker := func(name string) *gobreaker.CircuitBreaker {
		return gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        name,
			MaxRequests: 1,
			Interval:    10 * time.Second,
			Timeout:     15 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 3
			},
			OnStateChange: func(name string, from, to gobreaker.State) {
				slog.Warn("circuit breaker state change",
					"name", name, "from", from.String(), "to", to.String())
			},
		})
	}
	return &CircuitBreakerVectorStore{
		inner:         inner,
		storeBreaker:  breaker("qdrant-store"),
		searchBreaker: breaker("qdrant-search"),
		ensureBreaker: breaker("qdrant-ensure-collection"),
	}
}

func (c *CircuitBreakerVectorStore) Connect(ctx context.Context, dsn string) error {
	return c.inner.Connect(ctx, dsn)
}

func (c *CircuitBreakerVectorStore) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	_, err := c.ensureBreaker.Execute(func() (any, error) {
		return nil, c.inner.EnsureCollection(ctx, name, vectorSize, distance)
	})
	return err
}

func (c *CircuitBreakerVectorStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
	_, err := c.storeBreaker.Execute(func() (any, error) {
		return nil, c.inner.Store(ctx, collectionName, chunks)
	})
	return err
}

func (c *CircuitBreakerVectorStore) Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	result, err := c.searchBreaker.Execute(func() (any, error) {
		return c.inner.Search(ctx, collectionName, queryVector, topK)
	})
	if err != nil {
		return nil, err
	}
	results, ok := result.([]types.SearchResult)
	if !ok {
		return nil, fmt.Errorf("circuit breaker: unexpected result type %T", result)
	}
	return results, nil
}

func (c *CircuitBreakerVectorStore) ListCollections(ctx context.Context) ([]CollectionInfo, error) {
	result, err := c.searchBreaker.Execute(func() (any, error) {
		return c.inner.ListCollections(ctx)
	})
	if err != nil {
		return nil, err
	}
	collections, ok := result.([]CollectionInfo)
	if !ok {
		return nil, fmt.Errorf("circuit breaker: unexpected result type %T", result)
	}
	return collections, nil
}

func (c *CircuitBreakerVectorStore) GetCollection(ctx context.Context, name string) (*CollectionInfo, error) {
	result, err := c.searchBreaker.Execute(func() (any, error) {
		return c.inner.GetCollection(ctx, name)
	})
	if err != nil {
		return nil, err
	}
	info, ok := result.(*CollectionInfo)
	if !ok {
		return nil, fmt.Errorf("circuit breaker: unexpected result type %T", result)
	}
	return info, nil
}

func (c *CircuitBreakerVectorStore) DeleteCollection(ctx context.Context, name string) error {
	_, err := c.storeBreaker.Execute(func() (any, error) {
		return nil, c.inner.DeleteCollection(ctx, name)
	})
	return err
}

func (c *CircuitBreakerVectorStore) HealthCheck(ctx context.Context) error {
	return c.inner.HealthCheck(ctx)
}

func (c *CircuitBreakerVectorStore) Close() error {
	return c.inner.Close()
}
