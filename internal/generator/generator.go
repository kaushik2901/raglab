package generator

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

type Generator interface {
	Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	ModelName() string
}

type openAIGenerator struct {
	client openai.Client
	model  string
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
		client: openai.NewClient(opts...),
		model:  model,
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

	completion, err := g.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}
	return completion, nil
}

func (g *openAIGenerator) ModelName() string {
	return g.model
}
