package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockVectorStore struct {
	connectFn          func(ctx context.Context, dsn string) error
	ensureCollectionFn func(ctx context.Context, name string, vectorSize int, distance string) error
	storeFn            func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error
	searchFn           func(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error)
	listCollectionsFn  func(ctx context.Context) ([]CollectionInfo, error)
	getCollectionFn    func(ctx context.Context, name string) (*CollectionInfo, error)
	deleteCollectionFn func(ctx context.Context, name string) error
	healthCheckFn      func(ctx context.Context) error
	closeFn            func() error
}

func (m *mockVectorStore) Connect(ctx context.Context, dsn string) error {
	if m.connectFn != nil {
		return m.connectFn(ctx, dsn)
	}
	return nil
}

func (m *mockVectorStore) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	if m.ensureCollectionFn != nil {
		return m.ensureCollectionFn(ctx, name, vectorSize, distance)
	}
	return nil
}

func (m *mockVectorStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
	if m.storeFn != nil {
		return m.storeFn(ctx, collectionName, chunks)
	}
	return nil
}

func (m *mockVectorStore) Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, collectionName, queryVector, topK)
	}
	return nil, nil
}

func (m *mockVectorStore) HealthCheck(ctx context.Context) error {
	if m.healthCheckFn != nil {
		return m.healthCheckFn(ctx)
	}
	return nil
}

func (m *mockVectorStore) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func (m *mockVectorStore) ListCollections(ctx context.Context) ([]CollectionInfo, error) {
	if m.listCollectionsFn != nil {
		return m.listCollectionsFn(ctx)
	}
	return nil, nil
}

func (m *mockVectorStore) GetCollection(ctx context.Context, name string) (*CollectionInfo, error) {
	if m.getCollectionFn != nil {
		return m.getCollectionFn(ctx, name)
	}
	return nil, nil
}

func (m *mockVectorStore) DeleteCollection(ctx context.Context, name string) error {
	if m.deleteCollectionFn != nil {
		return m.deleteCollectionFn(ctx, name)
	}
	return nil
}

func TestCircuitBreakerStore_StoreClosed(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockVectorStore{
		storeFn: func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
			callCount.Add(1)
			return nil
		},
	}
	cb := NewCircuitBreakerVectorStore(inner)
	for i := 0; i < 5; i++ {
		err := cb.Store(context.Background(), "test", nil)
		require.NoError(t, err)
	}
	assert.Equal(t, int32(5), callCount.Load())
}

func TestCircuitBreakerStore_SearchClosed(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockVectorStore{
		searchFn: func(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			callCount.Add(1)
			return []types.SearchResult{{Score: 0.95}}, nil
		},
	}
	cb := NewCircuitBreakerVectorStore(inner)
	for i := 0; i < 5; i++ {
		results, err := cb.Search(context.Background(), "test", nil, 5)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	}
	assert.Equal(t, int32(5), callCount.Load())
}

func TestCircuitBreakerStore_StoreTripsIndependently(t *testing.T) {
	storeCount := atomic.Int32{}
	searchCount := atomic.Int32{}

	inner := &mockVectorStore{
		storeFn: func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
			storeCount.Add(1)
			return errors.New("store failed")
		},
		searchFn: func(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			searchCount.Add(1)
			return []types.SearchResult{{Score: 0.95}}, nil
		},
	}
	cb := NewCircuitBreakerVectorStore(inner)

	for i := 0; i < 4; i++ {
		cb.Store(context.Background(), "test", nil)
	}

	assert.Equal(t, int32(4), storeCount.Load())

	err := cb.Store(context.Background(), "test", nil)
	assert.Error(t, err)
	assert.Equal(t, int32(4), storeCount.Load(), "store should fail fast")

	results, err := cb.Search(context.Background(), "test", nil, 5)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, int32(1), searchCount.Load(), "search should still work")
}

func TestCircuitBreakerStore_EnsureAndBypassMethods(t *testing.T) {
	ensureCount := atomic.Int32{}
	connectCount := atomic.Int32{}
	healthCount := atomic.Int32{}
	closeCount := atomic.Int32{}

	inner := &mockVectorStore{
		ensureCollectionFn: func(ctx context.Context, name string, vectorSize int, distance string) error {
			ensureCount.Add(1)
			return errors.New("ensure failed")
		},
		connectFn: func(ctx context.Context, dsn string) error {
			connectCount.Add(1)
			return nil
		},
		healthCheckFn: func(ctx context.Context) error {
			healthCount.Add(1)
			return nil
		},
		closeFn: func() error {
			closeCount.Add(1)
			return nil
		},
	}
	cb := NewCircuitBreakerVectorStore(inner)

	for i := 0; i < 4; i++ {
		cb.EnsureCollection(context.Background(), "test", 4, "Cosine")
	}
	assert.Equal(t, int32(4), ensureCount.Load())

	err := cb.EnsureCollection(context.Background(), "test", 4, "Cosine")
	assert.Error(t, err)
	assert.Equal(t, int32(4), ensureCount.Load(), "ensure should fail fast when open")

	err = cb.Connect(context.Background(), "http://localhost:6334")
	require.NoError(t, err)
	assert.Equal(t, int32(1), connectCount.Load())

	err = cb.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), healthCount.Load())

	err = cb.Close()
	require.NoError(t, err)
	assert.Equal(t, int32(1), closeCount.Load())
}

func TestCircuitBreakerStore_HalfOpenStoreRecovery(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockVectorStore{
		storeFn: func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
			n := callCount.Add(1)
			if n < 5 {
				return errors.New("fail")
			}
			return nil
		},
	}

	cb := &CircuitBreakerVectorStore{
		inner: inner,
		storeBreaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "test-halfopen",
			MaxRequests: 1,
			Interval:    0,
			Timeout:     50 * time.Millisecond,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 3
			},
		}),
		searchBreaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "test-search",
		}),
		ensureBreaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "test-ensure",
		}),
	}

	for i := 0; i < 4; i++ {
		cb.Store(context.Background(), "test", nil)
	}

	err := cb.Store(context.Background(), "test", nil)
	assert.Error(t, err)

	time.Sleep(60 * time.Millisecond)

	err = cb.Store(context.Background(), "test", nil)
	require.NoError(t, err)
	assert.Equal(t, int32(5), callCount.Load())
}

func TestCircuitBreakerStore_ConcurrentAccess(t *testing.T) {
	inner := &mockVectorStore{
		storeFn: func(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
			return nil
		},
	}
	cb := NewCircuitBreakerVectorStore(inner)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Store(context.Background(), "test", nil)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}
