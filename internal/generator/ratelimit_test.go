package generator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGenerator struct {
	generateFn  func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	modelNameFn func() string
}

func (m *mockGenerator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, params)
	}
	return &openai.ChatCompletion{}, nil
}

func (m *mockGenerator) ModelName() string {
	if m.modelNameFn != nil {
		return m.modelNameFn()
	}
	return "mock"
}

func TestRateLimitedGenerator_Delegation(t *testing.T) {
	expectedParams := openai.ChatCompletionNewParams{
		Model: openai.ChatModel("gpt-4"),
	}
	expectedResult := &openai.ChatCompletion{
		ID: "chatcmpl-123",
	}

	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			assert.Equal(t, "gpt-4", string(params.Model), "model must be forwarded")
			return expectedResult, nil
		},
		modelNameFn: func() string { return "gpt-4" },
	}

	rl := NewRateLimitedGenerator(inner, 100000)
	t.Run("generates and delegates", func(t *testing.T) {
		result, err := rl.Generate(context.Background(), expectedParams)
		require.NoError(t, err)
		assert.Equal(t, expectedResult, result)
	})
	t.Run("model name delegates", func(t *testing.T) {
		assert.Equal(t, "gpt-4", rl.ModelName())
	})
}

func TestRateLimitedGenerator_HighRPM(t *testing.T) {
	callCount := atomic.Int32{}
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			callCount.Add(1)
			return &openai.ChatCompletion{}, nil
		},
	}
	rl := NewRateLimitedGenerator(inner, 100000)

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		_, err := rl.Generate(ctx, openai.ChatCompletionNewParams{})
		require.NoError(t, err)
	}
	assert.Equal(t, int32(10), callCount.Load(), "all 10 calls should succeed")
}

func TestRateLimitedGenerator_RateLimitBlocks(t *testing.T) {
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{}, nil
		},
	}
	rl := NewRateLimitedGenerator(inner, 0.5)

	ctx := context.Background()

	_, err := rl.Generate(ctx, openai.ChatCompletionNewParams{})
	require.NoError(t, err)

	shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_, err = rl.Generate(shortCtx, openai.ChatCompletionNewParams{})
	assert.Error(t, err, "second call should block and eventually timeout")
}

func TestRateLimitedGenerator_CancelDuringWait(t *testing.T) {
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{}, nil
		},
	}
	rl := NewRateLimitedGenerator(inner, 0.001)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rl.Generate(ctx, openai.ChatCompletionNewParams{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRateLimitedGenerator_ZeroRPM(t *testing.T) {
	inner := &mockGenerator{}
	rl := NewRateLimitedGenerator(inner, 0)
	assert.Same(t, inner, rl, "should return inner when RPM is 0")
}

func TestRateLimitedGenerator_NegativeRPM(t *testing.T) {
	inner := &mockGenerator{}
	rl := NewRateLimitedGenerator(inner, -1)
	assert.Same(t, inner, rl, "should return inner when RPM is negative")
}

func TestRateLimitedGenerator_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("generation failed")
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, expectedErr
		},
	}
	rl := NewRateLimitedGenerator(inner, 100000)

	_, err := rl.Generate(context.Background(), openai.ChatCompletionNewParams{})
	assert.ErrorIs(t, err, expectedErr)
}

func TestRateLimitedGenerator_ConcurrentAccess(t *testing.T) {
	inner := &mockGenerator{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{}, nil
		},
	}
	rl := NewRateLimitedGenerator(inner, 100000)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rl.Generate(context.Background(), openai.ChatCompletionNewParams{})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}
