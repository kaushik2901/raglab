package generator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

type Generator interface {
	Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	ModelName() string
}

type openAIGenerator struct {
	client           openai.Client
	model            string
	retryMaxAttempts int
	retryBackoff     time.Duration
}

func NewOpenAI(baseURL, apiKey, model string) Generator {
	config.WarnOnInsecure(baseURL, apiKey, "generator")
	baseURL = config.NormalizeBaseURL(baseURL)
	opts := []option.RequestOption{
		option.WithBaseURL(baseURL + "/v1/"),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	opts = append(opts, option.WithMaxRetries(0))
	return &openAIGenerator{
		client:           openai.NewClient(opts...),
		model:            model,
		retryMaxAttempts: 5,
		retryBackoff:     200 * time.Millisecond,
	}
}

// New creates a Generator for the given provider and model.
// It resolves the provider-specific base URL and API key from environment variables.
func New(provider config.Provider, model string) (Generator, error) {
	baseURL, apiKey := config.ResolveProviderConfig(provider)
	if baseURL == "" {
		return nil, fmt.Errorf("empty base URL for provider %q", provider)
	}
	gen := NewOpenAI(baseURL, apiKey, model)
	rpm := config.FloatEnvOrDefault("GENERATOR_RATE_LIMIT_RPM", 100)
	return NewRateLimitedGenerator(gen, rpm), nil
}

func (g *openAIGenerator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	params.Model = openai.ChatModel(g.model)

	for attempt := 0; attempt <= g.retryMaxAttempts; attempt++ {
		completion, err := g.client.Chat.Completions.New(ctx, params)
		if err == nil {
			return completion, nil
		}

		var apiErr *openai.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests && attempt < g.retryMaxAttempts {
			backoff := g.retryBackoff * (1 << attempt)
			if apiErr.Response != nil {
				if retryAfter := config.ParseRetryAfter(apiErr.Response.Header.Get("Retry-After")); retryAfter > backoff {
					backoff = retryAfter
				}
			}
			jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
			slog.Warn("rate limit hit, retrying", "attempt", attempt+1, "backoff", backoff+jitter)
			time.Sleep(backoff + jitter)
			continue
		}

		return nil, fmt.Errorf("chat completion: %w", err)
	}

	slog.Warn("rate limit retries exhausted", "max_attempts", g.retryMaxAttempts, "model", g.model)
	return nil, fmt.Errorf("rate limit exceeded after %d retries", g.retryMaxAttempts)
}

func (g *openAIGenerator) ModelName() string {
	return g.model
}
