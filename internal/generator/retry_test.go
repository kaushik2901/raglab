package generator

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
)

func TestRetryGenerator_Generate_Success(t *testing.T) {
	expected := &openai.ChatCompletion{ID: "cmpl-1"}
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return expected, nil
		},
	}
	r := NewRetryGenerator(inner)
	result, err := r.Generate(context.Background(), openai.ChatCompletionNewParams{})
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestRetryGenerator_Generate_RetryThenSuccess(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			n := callCount.Add(1)
			if n < 3 {
				return nil, &openai.Error{StatusCode: http.StatusTooManyRequests}
			}
			return &openai.ChatCompletion{ID: "cmpl-1"}, nil
		},
	}
	r := NewRetryGenerator(inner)
	result, err := r.Generate(context.Background(), openai.ChatCompletionNewParams{})
	require.NoError(t, err)
	assert.Equal(t, "cmpl-1", result.ID)
	assert.Equal(t, int32(3), callCount.Load())
}

func TestRetryGenerator_Generate_ExhaustRetries(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			callCount.Add(1)
			return nil, &openai.Error{StatusCode: http.StatusServiceUnavailable}
		},
	}
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = time.Millisecond
	b.MaxInterval = 5 * time.Millisecond
	b.MaxElapsedTime = 50 * time.Millisecond
	r := NewRetryGeneratorWithBackOff(inner, b)
	_, err := r.Generate(context.Background(), openai.ChatCompletionNewParams{})
	assert.Error(t, err)
	assert.GreaterOrEqual(t, callCount.Load(), int32(2))
}

func TestRetryGenerator_Generate_NonRetryableError(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			callCount.Add(1)
			return nil, errors.New("bad request")
		},
	}
	r := NewRetryGenerator(inner)
	_, err := r.Generate(context.Background(), openai.ChatCompletionNewParams{})
	assert.Error(t, err)
	assert.Equal(t, int32(1), callCount.Load())
}

func TestRetryGenerator_Generate_ContextCancel(t *testing.T) {
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, &openai.Error{StatusCode: http.StatusTooManyRequests}
		},
	}
	r := NewRetryGenerator(inner)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Generate(ctx, openai.ChatCompletionNewParams{})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRetryGenerator_Generate_ContextDeadline(t *testing.T) {
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, &openai.Error{StatusCode: http.StatusServiceUnavailable}
		},
	}
	r := NewRetryGenerator(inner)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := r.Generate(ctx, openai.ChatCompletionNewParams{})
	assert.Error(t, err)
}

func TestRetryGenerator_Generate_RetryableNetworkErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"connection refused", syscall.ECONNREFUSED},
		{"connection reset", syscall.ECONNRESET},
		{"i/o timeout", syscall.ETIMEDOUT},
		{"unexpected EOF", io.ErrUnexpectedEOF},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			callCount := atomic.Int32{}
			inner := &mockGenerator{
				generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
					n := callCount.Add(1)
					if n < 2 {
						return nil, tc.err
					}
					return &openai.ChatCompletion{ID: "cmpl-1"}, nil
				},
			}
			r := NewRetryGenerator(inner)
			_, err := r.Generate(context.Background(), openai.ChatCompletionNewParams{})
			require.NoError(t, err)
			assert.Equal(t, int32(2), callCount.Load())
		})
	}
}

func TestRetryGenerator_Generate_HTTP5xxRetryable(t *testing.T) {
	codes := []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			callCount := atomic.Int32{}
			inner := &mockGenerator{
				generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
					n := callCount.Add(1)
					if n < 2 {
						return nil, &openai.Error{StatusCode: code}
					}
					return &openai.ChatCompletion{ID: "cmpl-1"}, nil
				},
			}
			r := NewRetryGenerator(inner)
			_, err := r.Generate(context.Background(), openai.ChatCompletionNewParams{})
			require.NoError(t, err)
			assert.Equal(t, int32(2), callCount.Load())
		})
	}
}

func TestRetryGenerator_GenerateStream_Passthrough(t *testing.T) {
	expected := &openai.ChatCompletion{ID: "stream-cmpl"}
	inner := &mockGenerator{
		generateStreamFn: func(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error) {
			return expected, nil
		},
	}
	r := NewRetryGenerator(inner)
	result, err := r.GenerateStream(context.Background(), openai.ChatCompletionNewParams{}, nil)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestRetryGenerator_DelegatesModelName(t *testing.T) {
	inner := &mockGenerator{
		modelNameFn: func() string { return "gpt-4" },
	}
	r := NewRetryGenerator(inner)
	assert.Equal(t, "gpt-4", r.ModelName())
}

func TestRetryGenerator_Generate_EmptyResult(t *testing.T) {
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{}, nil
		},
	}
	r := NewRetryGenerator(inner)
	result, err := r.Generate(context.Background(), openai.ChatCompletionNewParams{})
	require.NoError(t, err)
	assert.NotNil(t, result)
}
