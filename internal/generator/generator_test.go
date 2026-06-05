package generator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenAI(t *testing.T) {
	g := NewOpenAI("https://api.openai.com/v1", "sk-test", "gpt-4o-mini")
	assert.NotNil(t, g)
	assert.Equal(t, "gpt-4o-mini", g.ModelName())
}

func TestNewEmptyAPIKey(t *testing.T) {
	g := NewOpenAI("http://localhost:1234/v1", "", "local-model")
	assert.NotNil(t, g)
	assert.Equal(t, "local-model", g.ModelName())
}

func TestGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello, world!",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := NewOpenAI(srv.URL, "", "gpt-4o-mini")
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("say hello"),
		},
	}

	completion, err := g.Generate(context.Background(), params)
	require.NoError(t, err)
	require.Len(t, completion.Choices, 1)
	assert.Equal(t, "Hello, world!", completion.Choices[0].Message.Content)
	assert.Equal(t, int64(10), completion.Usage.PromptTokens)
	assert.Equal(t, int64(20), completion.Usage.CompletionTokens)
}

func TestGenerate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	g := NewOpenAI(srv.URL, "", "gpt-4o-mini")
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hello"),
		},
	}

	_, err := g.Generate(context.Background(), params)
	require.Error(t, err)
}

func TestGenerate_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{},
			"usage": map[string]any{
				"prompt_tokens":     0,
				"completion_tokens": 0,
				"total_tokens":      0,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := NewOpenAI(srv.URL, "", "gpt-4o-mini")
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hello"),
		},
	}

	completion, err := g.Generate(context.Background(), params)
	require.NoError(t, err)
	assert.Empty(t, completion.Choices)
}

func TestGenerate_RateLimitRetry_Success(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&attempts, 1)
		if cur == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"rate limit","type":"rate_limit_error","code":null,"param":null}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message":       map[string]any{"role": "assistant", "content": "Hello!"},
				},
			},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer srv.Close()

	g := NewOpenAI(srv.URL, "", "gpt-4o-mini")
	gen := g.(*openAIGenerator)
	gen.retryBackoff = time.Millisecond
	gen.retryMaxAttempts = 3

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hello"),
		},
	}

	completion, err := g.Generate(context.Background(), params)
	require.NoError(t, err)
	require.Len(t, completion.Choices, 1)
	assert.Equal(t, "Hello!", completion.Choices[0].Message.Content)
	assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
}

func TestGenerate_RateLimitRetry_Exhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limit","type":"rate_limit_error","code":null,"param":null}}`))
	}))
	defer srv.Close()

	g := NewOpenAI(srv.URL, "", "gpt-4o-mini")
	gen := g.(*openAIGenerator)
	gen.retryBackoff = time.Millisecond
	gen.retryMaxAttempts = 2

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hello"),
		},
	}

	_, err := g.Generate(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.openai.com/v1", "https://api.openai.com"},
		{"https://api.openai.com/v1/", "https://api.openai.com"},
		{"https://api.openai.com", "https://api.openai.com"},
		{"http://localhost:1234/v1", "http://localhost:1234"},
		{"http://localhost:1234/v1/", "http://localhost:1234"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeBaseURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
