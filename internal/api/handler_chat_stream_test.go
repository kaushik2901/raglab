package api

import (
	"bufio"
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"

	"github.com/kaushik2901/raglab/internal/embedder"
	"github.com/kaushik2901/raglab/internal/generator"
	"github.com/kaushik2901/raglab/internal/types"
)

type mockRetrieverForStream struct {
	retrieveFn func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error)
}

func (m *mockRetrieverForStream) Retrieve(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	return m.retrieveFn(ctx, collection, queryVector, topK)
}

type mockEmbedderForStream struct{}

func (m *mockEmbedderForStream) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	return []types.Embedding{{Vector: []float64{0.1, 0.2}}}, nil
}
func (m *mockEmbedderForStream) Dimensions() int   { return 768 }
func (m *mockEmbedderForStream) ModelName() string { return "mock" }

type mockGenForStream struct {
	generateStreamFn func(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error)
}

func (m *mockGenForStream) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return &openai.ChatCompletion{}, nil
}
func (m *mockGenForStream) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error) {
	return m.generateStreamFn(ctx, params, cb)
}
func (m *mockGenForStream) ModelName() string { return "mock" }

func newTestChatServiceWithMocks(mockRet retrieverInterface, mockGen generator.Generator) *ChatService {
	return &ChatService{
		newEmbedderFn: func(req ChatRequest) (embedder.Embedder, error) {
			return &mockEmbedderForStream{}, nil
		},
		newRetrieverFn: func() (retrieverInterface, error) {
			return mockRet, nil
		},
		newGeneratorFn: func(req ChatRequest) (generator.Generator, error) {
			return mockGen, nil
		},
	}
}

func TestChatStreamHandler_Success(t *testing.T) {
	mockRet := &mockRetrieverForStream{
		retrieveFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return []types.SearchResult{
				{DocumentPath: "doc1.md", Content: "content", Score: 0.95},
			}, nil
		},
	}
	mockGen := &mockGenForStream{
		generateStreamFn: func(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error) {
			cb("Hello")
			cb(" ")
			cb("world")
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: "Hello world"}},
				},
				Usage: openai.CompletionUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			}, nil
		},
	}
	cr := NewChatRouter(newTestChatServiceWithMocks(mockRet, mockGen))

	body := `{"tag": "test-collection", "query": "test query", "top_k": 5, "max_tokens": 1024, "temperature": 0.3, "llm_provider": "openai", "llm_model": "gpt-4o-mini", "embedding_provider": "openai", "embedding_model": "text-embedding-3-small"}`
	req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	cr.chatStreamHandler(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))

	scanner := bufio.NewScanner(rec.Body)
	var parts []map[string]any
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var part map[string]any
			if err := jsonUnmarshal([]byte(data), &part); err == nil {
				parts = append(parts, part)
			}
		}
	}

	var sourceCount, deltaCount int
	var hasTextEnd bool
	for _, p := range parts {
		switch p["type"] {
		case "source-document":
			sourceCount++
			assert.Equal(t, "doc1.md", p["sourceId"])
		case "text-delta":
			deltaCount++
			assert.Contains(t, p, "id")
			assert.Contains(t, p, "delta")
		case "text-end":
			hasTextEnd = true
			assert.Contains(t, p, "id")
		}
	}
	assert.Equal(t, 1, sourceCount)
	assert.Equal(t, 3, deltaCount)
	assert.True(t, hasTextEnd)
}

func TestChatStreamHandler_InvalidJSON(t *testing.T) {
	cr := NewChatRouter(nil)

	body := `{bad json`
	req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	cr.chatStreamHandler(rec, req)

	assert.Equal(t, 400, rec.Code)
	var p ProblemDetail
	jsonUnmarshal(rec.Body.Bytes(), &p)
	assert.Equal(t, "Invalid Request Body", p.Title)
}

func TestChatStreamHandler_MissingFields(t *testing.T) {
	cr := NewChatRouter(nil)

	tests := []struct {
		name string
		body string
	}{
		{"missing query", `{"tag": "col"}`},
		{"missing tag", `{"query": "q"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			cr.chatStreamHandler(rec, req)

			assert.Equal(t, 400, rec.Code)
		})
	}
}

func TestChatStreamHandler_RetrievalError(t *testing.T) {
	mockRet := &mockRetrieverForStream{
		retrieveFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return nil, fmt.Errorf("qdrant error")
		},
	}
	cr := NewChatRouter(newTestChatServiceWithMocks(mockRet, &mockGenForStream{}))

	body := `{"tag": "col", "query": "test", "top_k": 5, "max_tokens": 1024, "temperature": 0.3, "llm_provider": "openai", "llm_model": "gpt-4o-mini", "embedding_provider": "openai", "embedding_model": "text-embedding-3-small"}`
	req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	cr.chatStreamHandler(rec, req)

	scanner := bufio.NewScanner(rec.Body)
	var hasError bool
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "RETRIEVAL_FAILED") {
			hasError = true
		}
	}
	assert.True(t, hasError)
}

func TestChatStreamHandler_WithMessages(t *testing.T) {
	mockRet := &mockRetrieverForStream{
		retrieveFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return []types.SearchResult{{DocumentPath: "doc1.md", Content: "content", Score: 0.95}}, nil
		},
	}
	mockGen := &mockGenForStream{
		generateStreamFn: func(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error) {
			cb("Hello world")
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: "Hello world"}},
				},
				Usage: openai.CompletionUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			}, nil
		},
	}
	cr := NewChatRouter(newTestChatServiceWithMocks(mockRet, mockGen))

	body := `{"tag": "col", "messages": [{"role":"user","content":"previous question"},{"role":"assistant","content":"previous answer"},{"role":"user","content":"new question"}], "top_k": 5, "max_tokens": 1024, "temperature": 0.3, "llm_provider": "openai", "llm_model": "gpt-4o-mini", "embedding_provider": "openai", "embedding_model": "text-embedding-3-small"}`
	req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	cr.chatStreamHandler(rec, req)

	assert.Equal(t, 200, rec.Code)
	scanner := bufio.NewScanner(rec.Body)
	var hasTextStart, hasTextEnd bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			if strings.Contains(line, `"type":"text-start"`) {
				hasTextStart = true
			}
			if strings.Contains(line, `"type":"text-end"`) {
				hasTextEnd = true
			}
		}
	}
	assert.True(t, hasTextStart)
	assert.True(t, hasTextEnd)
}
