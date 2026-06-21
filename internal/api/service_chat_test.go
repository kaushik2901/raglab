package api

import (
	"context"
	"errors"
	"testing"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/raglab/internal/embedder"
	"github.com/kaushik2901/raglab/internal/generator"
	"github.com/kaushik2901/raglab/internal/types"
)

type mockEmbedder struct {
	embedFn      func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error)
	dimensionsFn func() int
	modelNameFn  func() string
}

func (m *mockEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, chunks)
	}
	return make([]types.Embedding, len(chunks)), nil
}

func (m *mockEmbedder) Dimensions() int {
	if m.dimensionsFn != nil {
		return m.dimensionsFn()
	}
	return 768
}

func (m *mockEmbedder) ModelName() string {
	if m.modelNameFn != nil {
		return m.modelNameFn()
	}
	return "mock"
}

type mockGen struct {
	generateFn       func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	generateStreamFn func(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error)
}

func (m *mockGen) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, params)
	}
	return &openai.ChatCompletion{}, nil
}

func (m *mockGen) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error) {
	if m.generateStreamFn != nil {
		return m.generateStreamFn(ctx, params, cb)
	}
	return &openai.ChatCompletion{}, nil
}

func (m *mockGen) ModelName() string { return "mock" }

type mockRetrieverForService struct {
	retrieveFn func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error)
}

func (m *mockRetrieverForService) Retrieve(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	return m.retrieveFn(ctx, collection, queryVector, topK)
}

func newTestChatService() *ChatService {
	return &ChatService{}
}

func TestChatService_BuildMessages(t *testing.T) {
	t.Parallel()

	svc := newTestChatService()

	results := []types.SearchResult{
		{DocumentPath: "doc1.md", Content: "content one"},
		{DocumentPath: "doc2.md", Content: "content two"},
	}

	req := ChatRequest{
		Tag:               "col",
		Query:             "test question",
		TopK:              3,
		MaxTokens:         1024,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	msgs := svc.buildMessages(req, results)
	require.Len(t, msgs, 2) // system + user

	raw := msgs[1].GetContent().AsAny()
	content, ok := raw.(*string)
	require.True(t, ok, "expected content to be *string")
	require.NotNil(t, content)
	assert.Contains(t, *content, "doc1.md")
	assert.Contains(t, *content, "content one")
	assert.Contains(t, *content, "content two")
	assert.Contains(t, *content, "test question")
}

func TestChatService_BuildMessages_WithSourceURL(t *testing.T) {
	t.Parallel()

	svc := newTestChatService()

	results := []types.SearchResult{
		{
			DocumentPath: "doc1.md",
			Content:      "content one",
			Metadata:     map[string]string{"source_url": "https://example.com/doc1/"},
		},
		{
			DocumentPath: "doc2.md",
			Content:      "content two",
		},
	}

	req := ChatRequest{
		Tag:               "col",
		Query:             "test question",
		TopK:              3,
		MaxTokens:         1024,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	msgs := svc.buildMessages(req, results)
	require.Len(t, msgs, 2)

	raw := msgs[1].GetContent().AsAny()
	content, ok := raw.(*string)
	require.True(t, ok)

	assert.Contains(t, *content, "https://example.com/doc1/")
	assert.Contains(t, *content, "content one")
	assert.Contains(t, *content, "doc2.md")
	assert.Contains(t, *content, "content two")
	assert.NotContains(t, *content, "Document: doc1.md")
}

func TestChatService_BuildMessages_WithHistory(t *testing.T) {
	t.Parallel()

	svc := newTestChatService()

	req := ChatRequest{
		Tag:   "col",
		Query: "new question",
		Messages: []ChatMessage{
			{Role: "user", Content: "previous question"},
			{Role: "assistant", Content: "previous answer"},
			{Role: "user", Content: "new question"},
		},
		TopK:              3,
		MaxTokens:         1024,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	msgs := svc.buildMessages(req, []types.SearchResult{
		{DocumentPath: "doc1.md", Content: "content"},
	})
	require.Len(t, msgs, 4) // system, user(prev), assistant(prev), user(current with RAG)
}

func TestChatService_BuildMessages_QueryFromMessages(t *testing.T) {
	t.Parallel()

	svc := newTestChatService()

	req := ChatRequest{
		Tag: "col",
		Messages: []ChatMessage{
			{Role: "user", Content: "the question from messages"},
		},
		TopK:              3,
		MaxTokens:         1024,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	assert.Equal(t, "the question from messages", req.QueryText())

	msgs := svc.buildMessages(req, []types.SearchResult{
		{DocumentPath: "doc1.md", Content: "content"},
	})
	require.Len(t, msgs, 2) // system + user(RAG context)
	raw := msgs[1].GetContent().AsAny()
	content, ok := raw.(*string)
	require.True(t, ok)
	assert.Contains(t, *content, "the question from messages")
}

func TestChatService_Chat_Success(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
			newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
				return []types.Embedding{{Vector: []float64{0.1, 0.2}}}, nil
			}}, nil
		},
		newRetrieverFn: func() (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
					return []types.SearchResult{
						{DocumentPath: "doc1.md", Content: "relevant", Score: 0.95},
					}, nil
				},
			}, nil
		},
		newGeneratorFn: func(req ChatRequest) (generator.Generator, error) {
			return &mockGen{
				generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
					return &openai.ChatCompletion{
						Choices: []openai.ChatCompletionChoice{
							{Message: openai.ChatCompletionMessage{Content: "the answer"}},
						},
						Usage: openai.CompletionUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
					}, nil
				},
			}, nil
		},
	}

	req := ChatRequest{
		Tag:               "col",
		Query:             "test",
		TopK:              5,
		Temperature:       0.3,
		MaxTokens:         1024,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	resp, err := svc.Chat(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "the answer", resp.Answer)
	require.Len(t, resp.SourceDocuments, 1)
	assert.Equal(t, "doc1.md", resp.SourceDocuments[0].DocumentPath)
	assert.Equal(t, TokenUsage{Prompt: 10, Completion: 5, Total: 15}, resp.TokenUsage)
	assert.GreaterOrEqual(t, resp.LatencyMs, int64(0))
	assert.Equal(t, "", resp.SourceDocuments[0].SourceURL)
}

func TestChatService_Chat_SourceURLPopulated(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
			newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
				return []types.Embedding{{Vector: []float64{0.1, 0.2}}}, nil
			}}, nil
		},
		newRetrieverFn: func() (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
					return []types.SearchResult{
						{
							DocumentPath: "doc1.md",
							Content:      "relevant",
							Score:        0.95,
							Metadata:     map[string]string{"source_url": "https://example.com/doc1/"},
						},
					}, nil
				},
			}, nil
		},
		newGeneratorFn: func(req ChatRequest) (generator.Generator, error) {
			return &mockGen{
				generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
					return &openai.ChatCompletion{
						Choices: []openai.ChatCompletionChoice{
							{Message: openai.ChatCompletionMessage{Content: "answer"}},
						},
						Usage: openai.CompletionUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
					}, nil
				},
			}, nil
		},
	}

	req := ChatRequest{
		Tag:               "col",
		Query:             "test",
		TopK:              5,
		Temperature:       0.3,
		MaxTokens:         1024,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	resp, err := svc.Chat(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, resp.SourceDocuments, 1)
	assert.Equal(t, "doc1.md", resp.SourceDocuments[0].DocumentPath)
	assert.Equal(t, "https://example.com/doc1/", resp.SourceDocuments[0].SourceURL)
}

func TestChatService_Chat_RetrieverError(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
			newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
				return []types.Embedding{{Vector: []float64{0.1}}}, nil
			}}, nil
		},
		newRetrieverFn: func() (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
					return nil, errors.New("qdrant error")
				},
			}, nil
		},
	}

	req := ChatRequest{
		Tag:               "col",
		Query:             "test",
		TopK:              3,
		Temperature:       0.5,
		MaxTokens:         512,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	_, err := svc.Chat(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retrieve")
}

func TestChatService_Chat_GeneratorError(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
			newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
				return []types.Embedding{{Vector: []float64{0.1}}}, nil
			}}, nil
		},
		newRetrieverFn: func() (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
					return []types.SearchResult{{DocumentPath: "d.md", Content: "c", Score: 0.9}}, nil
				},
			}, nil
		},
		newGeneratorFn: func(req ChatRequest) (generator.Generator, error) {
			return &mockGen{
				generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
					return nil, errors.New("llm error")
				},
			}, nil
		},
	}

	req := ChatRequest{
		Tag:               "col",
		Query:             "test",
		TopK:              3,
		Temperature:       0.5,
		MaxTokens:         512,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	_, err := svc.Chat(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generate")
}
