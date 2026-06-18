package embedder

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

	"github.com/kaushik2901/raglab/internal/types"
)

func TestCircuitBreakerEmbedder_ClosedState(t *testing.T) {
	expected := []types.Embedding{{ChunkID: "c1"}}
	callCount := atomic.Int32{}
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			callCount.Add(1)
			return expected, nil
		},
	}
	cb := NewCircuitBreakerEmbedder(inner)
	for i := 0; i < 10; i++ {
		result, err := cb.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	}
	assert.Equal(t, int32(10), callCount.Load())
}

func TestCircuitBreakerEmbedder_TripsOnFailures(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			n := callCount.Add(1)
			if n < 7 {
				return nil, errors.New("service unavailable")
			}
			return []types.Embedding{{ChunkID: "c1"}}, nil
		},
	}
	cb := NewCircuitBreakerEmbedder(inner)

	for i := 0; i < 6; i++ {
		_, err := cb.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
		assert.Error(t, err)
	}
	assert.Equal(t, int32(6), callCount.Load(), "all 6 failures should reach inner")

	_, err := cb.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")
	assert.Equal(t, int32(6), callCount.Load(), "should NOT call inner when open")
}

func TestCircuitBreakerEmbedder_HalfOpenSuccess(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			n := callCount.Add(1)
			if n < 7 {
				return nil, errors.New("fail")
			}
			return []types.Embedding{{ChunkID: "c1"}}, nil
		},
	}
	cb := &CircuitBreakerEmbedder{
		inner: inner,
		breaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "test-halfopen",
			MaxRequests: 1,
			Interval:    0,
			Timeout:     50 * time.Millisecond,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5
			},
		}),
	}

	for i := 0; i < 6; i++ {
		cb.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	}

	_, err := cb.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")

	time.Sleep(60 * time.Millisecond)

	result, err := cb.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	require.NoError(t, err)
	assert.Equal(t, []types.Embedding{{ChunkID: "c1"}}, result)
	assert.Equal(t, int32(7), callCount.Load(), "half-open probe should succeed")
}

func TestCircuitBreakerEmbedder_DelegatesDimensions(t *testing.T) {
	inner := &mockEmbedder{
		dimensionsFn: func() int { return 768 },
	}
	cb := NewCircuitBreakerEmbedder(inner)
	assert.Equal(t, 768, cb.Dimensions())
}

func TestCircuitBreakerEmbedder_DelegatesModelName(t *testing.T) {
	inner := &mockEmbedder{
		modelNameFn: func() string { return "test" },
	}
	cb := NewCircuitBreakerEmbedder(inner)
	assert.Equal(t, "test", cb.ModelName())
}

func TestCircuitBreakerEmbedder_ConcurrentAccess(t *testing.T) {
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			return []types.Embedding{{ChunkID: "c1"}}, nil
		},
	}
	cb := NewCircuitBreakerEmbedder(inner)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cb.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}
