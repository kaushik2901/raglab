package generator

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type Generator struct {
	client openai.Client
	model  string
}

func New(baseURL, apiKey, model string) *Generator {
	baseURL = normalizeBaseURL(baseURL)
	opts := []option.RequestOption{
		option.WithBaseURL(baseURL + "/v1/"),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &Generator{
		client: openai.NewClient(opts...),
		model:  model,
	}
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

func (g *Generator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	params.Model = openai.ChatModel(g.model)

	completion, err := g.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}
	return completion, nil
}
