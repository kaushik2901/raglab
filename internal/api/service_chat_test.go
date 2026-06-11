package api

import (
	"context"
	"errors"
	"testing"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/memory"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
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
	retrieveFn func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error)
}

func (m *mockRetrieverForService) Retrieve(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
	return m.retrieveFn(ctx, collection, query, topK)
}

func TestChatService_BuildMessages(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		memory: memory.NewRingBuffer(10),
	}

	results := []types.SearchResult{
		{DocumentPath: "doc1.md", Content: "content one"},
		{DocumentPath: "doc2.md", Content: "content two"},
	}

	req := ChatRequest{
		Tag:       "col",
		Query:     "test question",
		TopK:      3,
		MaxTokens: 1024,
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

	svc := &ChatService{
		memory: memory.NewRingBuffer(10),
	}

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
		Tag:       "col",
		Query:     "test question",
		TopK:      3,
		MaxTokens: 1024,
	}

	msgs := svc.buildMessages(req, results)
	require.Len(t, msgs, 2)

	raw := msgs[1].GetContent().AsAny()
	content, ok := raw.(*string)
	require.True(t, ok)

	// Document with source_url uses URL as label
	assert.Contains(t, *content, "https://example.com/doc1/")
	assert.Contains(t, *content, "content one")
	// Document without source_url falls back to DocumentPath
	assert.Contains(t, *content, "doc2.md")
	assert.Contains(t, *content, "content two")
	// DocumentPath not used when source_url is present
	assert.NotContains(t, *content, "Document: doc1.md")
}

func TestChatService_BuildMessages_WithMemory(t *testing.T) {
	t.Parallel()

	mem := memory.NewRingBuffer(10)
	mem.Add("conv-1", "previous question", "previous answer")

	svc := &ChatService{memory: mem}

	req := ChatRequest{
		Tag:            "col",
		Query:          "new question",
		TopK:           3,
		ConversationID: "conv-1",
		MaxTokens:      1024,
	}

	msgs := svc.buildMessages(req, []types.SearchResult{
		{DocumentPath: "doc1.md", Content: "content"},
	})
	require.Len(t, msgs, 4) // system, user(prev), assistant(prev), user(current)
}

func TestChatService_Chat_Success(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		memory: memory.NewRingBuffer(10),
		newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
				return []types.Embedding{{Vector: []float64{0.1, 0.2}}}, nil
			}}, nil
		},
		newRetrieverFn: func(emb embedder.Embedder) (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
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
	assert.Equal(t, "", resp.SourceDocuments[0].SourceURL) // no metadata in mock
}

func TestChatService_Chat_SourceURLPopulated(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		memory: memory.NewRingBuffer(10),
		newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{embedFn: func(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
				return []types.Embedding{{Vector: []float64{0.1, 0.2}}}, nil
			}}, nil
		},
		newRetrieverFn: func(emb embedder.Embedder) (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
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

func TestChatService_Chat_WithMemory(t *testing.T) {
	t.Parallel()

	mem := memory.NewRingBuffer(10)
	svc := &ChatService{
		memory: mem,
		newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{}, nil
		},
		newRetrieverFn: func(emb embedder.Embedder) (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
					return []types.SearchResult{{DocumentPath: "doc1.md", Content: "x", Score: 0.9}}, nil
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
						Usage: openai.CompletionUsage{PromptTokens: 1, CompletionTokens: 1},
					}, nil
				},
			}, nil
		},
	}

	req := ChatRequest{
		Tag:               "col",
		Query:             "new question",
		TopK:              3,
		Temperature:       0.5,
		MaxTokens:         512,
		ConversationID:    "conv-1",
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}

	_, err := svc.Chat(context.Background(), req)
	require.NoError(t, err)

	turns := mem.Get("conv-1")
	require.Len(t, turns, 1)
	assert.Equal(t, "new question", turns[0].User.Content)
	assert.Contains(t, turns[0].Assistant.Content, "answer")
}

func TestChatService_Chat_RetrieverError(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		memory: memory.NewRingBuffer(10),
		newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{}, nil
		},
		newRetrieverFn: func(emb embedder.Embedder) (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
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
		memory: memory.NewRingBuffer(10),
		newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedder{}, nil
		},
		newRetrieverFn: func(emb embedder.Embedder) (retrieverInterface, error) {
			return &mockRetrieverForService{
				retrieveFn: func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
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
