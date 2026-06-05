package generator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
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
	baseURL = normalizeBaseURL(baseURL)
	opts := []option.RequestOption{
		option.WithBaseURL(baseURL + "/v1/"),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
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
	return NewOpenAI(baseURL, apiKey, model), nil
}

// normalizeBaseURL strips any trailing /v1 or /v1/ suffix so that all
// endpoint paths can be constructed as baseURL + "/v1/" + endpoint.
func normalizeBaseURL(baseURL string) string {
	for _, suffix := range []string{"/v1/", "/v1"} {
		if len(baseURL) >= len(suffix) && baseURL[len(baseURL)-len(suffix):] == suffix {
			return baseURL[:len(baseURL)-len(suffix)]
		}
	}
	return baseURL
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
				if retryAfter := parseRetryAfter(apiErr.Response.Header.Get("Retry-After")); retryAfter > backoff {
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

// parseRetryAfter parses the Retry-After header value and returns the duration to wait.
// The header can be an integer number of seconds or an HTTP-date.
func parseRetryAfter(val string) time.Duration {
	if val == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(val); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if t, err := time.Parse(time.RFC1123, val); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func (g *openAIGenerator) ModelName() string {
	return g.model
}
