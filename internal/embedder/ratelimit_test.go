package embedder

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockEmbedder struct {
	embedFn      func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error)
	dimensionsFn func() int
	modelNameFn  func() string
}

func (m *mockEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, chunks)
	}
	return nil, nil
}

func (m *mockEmbedder) Dimensions() int {
	if m.dimensionsFn != nil {
		return m.dimensionsFn()
	}
	return 0
}

func (m *mockEmbedder) ModelName() string {
	if m.modelNameFn != nil {
		return m.modelNameFn()
	}
	return "mock"
}

func TestRateLimitedEmbedder_Delegation(t *testing.T) {
	expectedChunks := []types.Chunk{{ID: "c1", Content: "hello"}}
	expectedResult := []types.Embedding{{ChunkID: "c1", Vector: []float64{0.1}}}

	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			assert.Equal(t, expectedChunks, chunks, "chunks must be forwarded")
			return expectedResult, nil
		},
		dimensionsFn: func() int { return 3 },
		modelNameFn:  func() string { return "test-model" },
	}

	rl := NewRateLimitedEmbedder(inner, 100000)
	t.Run("embeds and delegates", func(t *testing.T) {
		result, err := rl.Embed(context.Background(), expectedChunks)
		require.NoError(t, err)
		assert.Equal(t, expectedResult, result)
	})
	t.Run("dimensions delegates", func(t *testing.T) {
		assert.Equal(t, 3, rl.Dimensions())
	})
	t.Run("model name delegates", func(t *testing.T) {
		assert.Equal(t, "test-model", rl.ModelName())
	})
}

func TestRateLimitedEmbedder_HighRPM(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			callCount.Add(1)
			return []types.Embedding{{ChunkID: "c1"}}, nil
		},
	}
	rl := NewRateLimitedEmbedder(inner, 100000)

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		_, err := rl.Embed(ctx, []types.Chunk{{ID: "c1"}})
		require.NoError(t, err)
	}
	assert.Equal(t, int32(10), callCount.Load(), "all 10 calls should succeed")
}

func TestRateLimitedEmbedder_RateLimitBlocks(t *testing.T) {
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			return []types.Embedding{{ChunkID: "c1"}}, nil
		},
	}
	rl := NewRateLimitedEmbedder(inner, 0.5)

	ctx := context.Background()

	_, err := rl.Embed(ctx, []types.Chunk{{ID: "c1"}})
	require.NoError(t, err)

	shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_, err = rl.Embed(shortCtx, []types.Chunk{{ID: "c2"}})
	assert.Error(t, err, "second call should block and eventually timeout")
}

func TestRateLimitedEmbedder_CancelDuringWait(t *testing.T) {
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			return []types.Embedding{{ChunkID: "c1"}}, nil
		},
	}
	rl := NewRateLimitedEmbedder(inner, 0.001)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rl.Embed(ctx, []types.Chunk{{ID: "c1"}})
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRateLimitedEmbedder_ZeroRPM(t *testing.T) {
	inner := &mockEmbedder{}
	rl := NewRateLimitedEmbedder(inner, 0)
	assert.Same(t, inner, rl, "should return inner when RPM is 0")
}

func TestRateLimitedEmbedder_NegativeRPM(t *testing.T) {
	inner := &mockEmbedder{}
	rl := NewRateLimitedEmbedder(inner, -1)
	assert.Same(t, inner, rl, "should return inner when RPM is negative")
}

func TestRateLimitedEmbedder_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("embedding failed")
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			return nil, expectedErr
		},
	}
	rl := NewRateLimitedEmbedder(inner, 100000)

	_, err := rl.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	assert.ErrorIs(t, err, expectedErr)
}

func TestRateLimitedEmbedder_EmptyChunks(t *testing.T) {
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			assert.Empty(t, chunks)
			return nil, nil
		},
	}
	rl := NewRateLimitedEmbedder(inner, 100000)

	result, err := rl.Embed(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestRateLimitedEmbedder_ConcurrentAccess(t *testing.T) {
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			return []types.Embedding{{ChunkID: chunks[0].ID}}, nil
		},
	}
	rl := NewRateLimitedEmbedder(inner, 100000)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := rl.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}
