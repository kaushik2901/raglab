package embedder

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/raglab/internal/types"
)

func TestRetryEmbedder_Success(t *testing.T) {
	expected := []types.Embedding{{ChunkID: "c1", Vector: []float64{0.1}}}
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			return expected, nil
		},
	}
	r := NewRetryEmbedder(inner)
	result, err := r.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestRetryEmbedder_RetryThenSuccess(t *testing.T) {
	callCount := atomic.Int32{}
	expected := []types.Embedding{{ChunkID: "c1"}}
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			n := callCount.Add(1)
			if n < 3 {
				return nil, &openai.Error{StatusCode: http.StatusTooManyRequests}
			}
			return expected, nil
		},
	}
	r := NewRetryEmbedder(inner)
	result, err := r.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	require.NoError(t, err)
	assert.Equal(t, expected, result)
	assert.Equal(t, int32(3), callCount.Load())
}

func TestRetryEmbedder_ExhaustRetries(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			callCount.Add(1)
			return nil, &openai.Error{StatusCode: http.StatusServiceUnavailable}
		},
	}
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = time.Millisecond
	b.MaxInterval = 5 * time.Millisecond
	b.MaxElapsedTime = 50 * time.Millisecond
	r := NewRetryEmbedderWithBackOff(inner, b)
	_, err := r.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	assert.Error(t, err)
	assert.GreaterOrEqual(t, callCount.Load(), int32(2), "should have retried at least once")
}

func TestRetryEmbedder_NonRetryableError(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			callCount.Add(1)
			return nil, errors.New("bad request")
		},
	}
	r := NewRetryEmbedder(inner)
	_, err := r.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
	assert.Error(t, err)
	assert.Equal(t, int32(1), callCount.Load(), "should not retry non-retryable error")
}

func TestRetryEmbedder_ContextCancel(t *testing.T) {
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			return nil, &openai.Error{StatusCode: http.StatusTooManyRequests}
		},
	}
	r := NewRetryEmbedder(inner)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Embed(ctx, []types.Chunk{{ID: "c1"}})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRetryEmbedder_ContextDeadline(t *testing.T) {
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			return nil, &openai.Error{StatusCode: http.StatusServiceUnavailable}
		},
	}
	r := NewRetryEmbedder(inner)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := r.Embed(ctx, []types.Chunk{{ID: "c1"}})
	assert.Error(t, err)
}

func TestRetryEmbedder_RetryableNetworkErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"connection refused", syscall.ECONNREFUSED},
		{"connection reset", syscall.ECONNRESET},
		{"i/o timeout", syscall.ETIMEDOUT},
		{"unexpected EOF", io.ErrUnexpectedEOF},
		{"EOF", io.EOF},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			callCount := atomic.Int32{}
			inner := &mockEmbedder{
				embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
					n := callCount.Add(1)
					if n < 2 {
						return nil, tc.err
					}
					return []types.Embedding{{ChunkID: "c1"}}, nil
				},
			}
			r := NewRetryEmbedder(inner)
			_, err := r.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
			require.NoError(t, err)
			assert.Equal(t, int32(2), callCount.Load())
		})
	}
}

func TestRetryEmbedder_HTTP5xxRetryable(t *testing.T) {
	codes := []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			callCount := atomic.Int32{}
			inner := &mockEmbedder{
				embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
					n := callCount.Add(1)
					if n < 2 {
						return nil, &openai.Error{StatusCode: code}
					}
					return []types.Embedding{{ChunkID: "c1"}}, nil
				},
			}
			r := NewRetryEmbedder(inner)
			_, err := r.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
			require.NoError(t, err)
			assert.Equal(t, int32(2), callCount.Load())
		})
	}
}

func TestRetryEmbedder_NonRetryableHTTPCodes(t *testing.T) {
	codes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
	}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			callCount := atomic.Int32{}
			inner := &mockEmbedder{
				embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
					callCount.Add(1)
					return nil, &openai.Error{StatusCode: code}
				},
			}
			r := NewRetryEmbedder(inner)
			_, err := r.Embed(context.Background(), []types.Chunk{{ID: "c1"}})
			assert.Error(t, err)
			assert.Equal(t, int32(1), callCount.Load())
		})
	}
}

func TestRetryEmbedder_DelegatesDimensions(t *testing.T) {
	inner := &mockEmbedder{
		dimensionsFn: func() int { return 1536 },
	}
	r := NewRetryEmbedder(inner)
	assert.Equal(t, 1536, r.Dimensions())
}

func TestRetryEmbedder_DelegatesModelName(t *testing.T) {
	inner := &mockEmbedder{
		modelNameFn: func() string { return "test-model" },
	}
	r := NewRetryEmbedder(inner)
	assert.Equal(t, "test-model", r.ModelName())
}

func TestRetryEmbedder_EmptyChunks(t *testing.T) {
	inner := &mockEmbedder{
		embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
			assert.Empty(t, chunks)
			return nil, nil
		},
	}
	r := NewRetryEmbedder(inner)
	result, err := r.Embed(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}
