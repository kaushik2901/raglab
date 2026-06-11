package generator

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

type StreamCallback func(token string) error

type Generator interface {
	Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error)
	ModelName() string
}

type openAIGenerator struct {
	client openai.Client
	model  string
}

func NewOpenAI(baseURL, apiKey, model string) Generator {
	config.WarnOnInsecure(baseURL, apiKey, "generator")
	baseURL = config.NormalizeBaseURL(baseURL)
	opts := []option.RequestOption{
		option.WithBaseURL(baseURL + "/v1/"),
		option.WithHeader("Content-Type", "application/json"),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	opts = append(opts, option.WithMaxRetries(0))
	return &openAIGenerator{
		client: openai.NewClient(opts...),
		model:  model,
	}
}

// New creates a Generator for the given provider and model.
// It resolves the provider-specific base URL and API key from environment variables.
// The returned generator is wrapped with retry, circuit breaker, and rate limiter decorators.
func New(provider config.Provider, model string) (Generator, error) {
	baseURL, apiKey := config.ResolveProviderConfig(provider)
	if baseURL == "" {
		return nil, fmt.Errorf("empty base URL for provider %q", provider)
	}
	var gen Generator = NewOpenAI(baseURL, apiKey, model)

	gen = NewRetryGenerator(gen)
	gen = NewCircuitBreakerGenerator(gen)
	rpm := config.FloatEnvOrDefault("GENERATOR_RATE_LIMIT_RPM", 100)
	return NewRateLimitedGenerator(gen, rpm), nil
}

func (g *openAIGenerator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	params.Model = openai.ChatModel(g.model)
	completion, err := g.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}
	return completion, nil
}

func (g *openAIGenerator) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error) {
	params.Model = openai.ChatModel(g.model)
	stream := g.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var full strings.Builder
	var lastChunk openai.ChatCompletionChunk
	for stream.Next() {
		chunk := stream.Current()
		lastChunk = chunk
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				full.WriteString(choice.Delta.Content)
				if err := cb(choice.Delta.Content); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}

	usage := openai.CompletionUsage{
		PromptTokens:     lastChunk.Usage.PromptTokens,
		CompletionTokens: lastChunk.Usage.CompletionTokens,
		TotalTokens:      lastChunk.Usage.TotalTokens,
	}

	return &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: full.String()}},
		},
		Usage: usage,
	}, nil
}

func (g *openAIGenerator) ModelName() string {
	return g.model
}
