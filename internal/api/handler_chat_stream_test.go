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

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/memory"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockRetrieverForStream struct {
	retrieveFn func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error)
}

func (m *mockRetrieverForStream) Retrieve(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
	return m.retrieveFn(ctx, collection, query, topK)
}

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

func TestChatStreamHandler_Success(t *testing.T) {
	srv := &Server{
		chat: &ChatService{
			retriever: &mockRetrieverForStream{
				retrieveFn: func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
					return []types.SearchResult{
						{DocumentPath: "doc1.md", Content: "content", Score: 0.95},
					}, nil
				},
			},
			generator: &mockGenForStream{
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
			},
			memory: memory.NewRingBuffer(10),
		},
	}

	body := `{"tag": "test-collection", "query": "test query"}`
	req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.chatStreamHandler(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))

	scanner := bufio.NewScanner(rec.Body)
	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			events = append(events, strings.TrimPrefix(line, "event: "))
		}
	}

	assert.Equal(t, []string{"retrieval", "token", "token", "token", "done"}, events)
}

func TestChatStreamHandler_InvalidJSON(t *testing.T) {
	srv := &Server{}

	body := `{bad json`
	req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.chatStreamHandler(rec, req)

	assert.Equal(t, 400, rec.Code)
	var p ProblemDetail
	jsonUnmarshal(rec.Body.Bytes(), &p)
	assert.Equal(t, "Invalid Request Body", p.Title)
}

func TestChatStreamHandler_MissingFields(t *testing.T) {
	srv := &Server{}

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

			srv.chatStreamHandler(rec, req)

			assert.Equal(t, 400, rec.Code)
		})
	}
}

func TestChatStreamHandler_RetrievalError(t *testing.T) {
	srv := &Server{
		chat: &ChatService{
			retriever: &mockRetrieverForStream{
				retrieveFn: func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
					return nil, fmt.Errorf("qdrant error")
				},
			},
		},
	}

	body := `{"tag": "col", "query": "test"}`
	req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.chatStreamHandler(rec, req)

	scanner := bufio.NewScanner(rec.Body)
	var hasError bool
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "RETRIEVAL_FAILED") {
			hasError = true
		}
	}
	assert.True(t, hasError)
}

func TestChatStreamHandler_WithMemory(t *testing.T) {
	mem := memory.NewRingBuffer(10)
	mem.Add("conv-1", "previous question", "previous answer")

	srv := &Server{
		chat: &ChatService{
			retriever: &mockRetrieverForStream{
				retrieveFn: func(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
					return []types.SearchResult{{DocumentPath: "doc1.md", Content: "content", Score: 0.95}}, nil
				},
			},
			generator: &mockGenForStream{
				generateStreamFn: func(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error) {
					cb("Hello world")
					return &openai.ChatCompletion{
						Choices: []openai.ChatCompletionChoice{
							{Message: openai.ChatCompletionMessage{Content: "Hello world"}},
						},
						Usage: openai.CompletionUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
					}, nil
				},
			},
			memory: mem,
		},
	}

	body := `{"tag": "col", "query": "new question", "conversation_id": "conv-1"}`
	req := httptest.NewRequest("POST", "/api/v1/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.chatStreamHandler(rec, req)

	turns := mem.Get("conv-1")
	assert.Len(t, turns, 2)
	assert.Equal(t, "new question", turns[1].User.Content)
	assert.Contains(t, turns[1].Assistant.Content, "Hello world")
}
