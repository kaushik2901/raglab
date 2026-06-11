package generator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreakerGenerator_ClosedState(t *testing.T) {
	expected := &openai.ChatCompletion{ID: "cmpl-1"}
	callCount := atomic.Int32{}
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			callCount.Add(1)
			return expected, nil
		},
	}
	cb := NewCircuitBreakerGenerator(inner)
	for i := 0; i < 10; i++ {
		result, err := cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	}
	assert.Equal(t, int32(10), callCount.Load())
}

func TestCircuitBreakerGenerator_TripsOnFailures(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			callCount.Add(1)
			return nil, errors.New("fail")
		},
	}
	cb := NewCircuitBreakerGenerator(inner)

	for i := 0; i < 6; i++ {
		cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
	}
	assert.Equal(t, int32(6), callCount.Load())

	_, err := cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")
	assert.Equal(t, int32(6), callCount.Load(), "should NOT call inner when open")
}

func TestCircuitBreakerGenerator_GenerateAndStreamSeparate(t *testing.T) {
	generateCount := atomic.Int32{}
	streamCount := atomic.Int32{}

	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			generateCount.Add(1)
			return nil, errors.New("fail")
		},
		generateStreamFn: func(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error) {
			streamCount.Add(1)
			return &openai.ChatCompletion{}, nil
		},
	}
	cb := NewCircuitBreakerGenerator(inner)

	for i := 0; i < 6; i++ {
		cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
	}

	_, err := cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
	assert.Error(t, err)

	result, err := cb.GenerateStream(context.Background(), openai.ChatCompletionNewParams{}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int32(1), streamCount.Load(), "stream should still work despite generate breaker being open")
}

func TestCircuitBreakerGenerator_HalfOpenSuccess(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			n := callCount.Add(1)
			if n < 7 {
				return nil, errors.New("fail")
			}
			return &openai.ChatCompletion{ID: "cmpl-recovered"}, nil
		},
	}
	cb := &CircuitBreakerGenerator{
		inner: inner,
		generateBreaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
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
		cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
	}

	_, err := cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
	require.Error(t, err)

	time.Sleep(60 * time.Millisecond)

	result, err := cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
	require.NoError(t, err)
	assert.Equal(t, "cmpl-recovered", result.ID)
}

func TestCircuitBreakerGenerator_DelegatesModelName(t *testing.T) {
	inner := &mockGenerator{
		modelNameFn: func() string { return "gpt-4" },
	}
	cb := NewCircuitBreakerGenerator(inner)
	assert.Equal(t, "gpt-4", cb.ModelName())
}

func TestCircuitBreakerGenerator_ConcurrentAccess(t *testing.T) {
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{}, nil
		},
	}
	cb := NewCircuitBreakerGenerator(inner)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cb.Generate(context.Background(), openai.ChatCompletionNewParams{})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}
