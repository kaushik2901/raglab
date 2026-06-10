package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/memory"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/retriever"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type retrieverInterface interface {
	Retrieve(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error)
}

type ChatService struct {
	vs     qstore.VectorStore
	cfg    *config.Config
	memory memory.Memory

	// Injectable factories for testing
	newEmbedderFn func(req ChatRequest) (embedder.Embedder, error)
	newRetrieverFn func(emb embedder.Embedder) (retrieverInterface, error)
	newGeneratorFn func(req ChatRequest) (generator.Generator, error)
}

func NewChatService(cfg *config.Config, vs qstore.VectorStore) (*ChatService, error) {
	mem := memory.NewRingBuffer(cfg.ChatMemorySize)
	s := &ChatService{cfg: cfg, vs: vs, memory: mem}
	s.newEmbedderFn = func(req ChatRequest) (embedder.Embedder, error) {
		return embedder.New(config.Provider(req.EmbeddingProvider), req.EmbeddingModel, 1)
	}
	s.newRetrieverFn = func(emb embedder.Embedder) (retrieverInterface, error) {
		return retriever.New(emb, s.vs, retriever.StrategyNaiveSearch)
	}
	s.newGeneratorFn = func(req ChatRequest) (generator.Generator, error) {
		return generator.New(config.Provider(req.LLMProvider), req.LLMModel)
	}
	return s, nil
}

func (s *ChatService) retrieveSources(ctx context.Context, req ChatRequest) ([]types.SearchResult, []SourceDoc, error) {
	emb, err := s.newEmbedderFn(req)
	if err != nil {
		return nil, nil, fmt.Errorf("create embedder: %w", err)
	}
	ret, err := s.newRetrieverFn(emb)
	if err != nil {
		return nil, nil, fmt.Errorf("create retriever: %w", err)
	}

	results, err := ret.Retrieve(ctx, req.Tag, req.Query, req.TopK)
	if err != nil {
		return nil, nil, fmt.Errorf("retrieve: %w", err)
	}

	sources := make([]SourceDoc, len(results))
	for i, r := range results {
		sources[i] = SourceDoc{DocumentPath: r.DocumentPath, Score: r.Score}
	}
	return results, sources, nil
}

func (s *ChatService) buildMessages(req ChatRequest, results []types.SearchResult) []openai.ChatCompletionMessageParamUnion {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a helpful assistant that answers questions based on the provided context. If the context does not contain enough information to answer, say so."),
	}

	if req.ConversationID != "" {
		for _, turn := range s.memory.Get(req.ConversationID) {
			messages = append(messages, openai.UserMessage(turn.User.Content))
			messages = append(messages, openai.AssistantMessage(turn.Assistant.Content))
		}
	}

	var contextParts []string
	for _, r := range results {
		contextParts = append(contextParts, fmt.Sprintf("Document: %s\n%s", r.DocumentPath, r.Content))
	}
	userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", strings.Join(contextParts, "\n\n"), req.Query)
	messages = append(messages, openai.UserMessage(userPrompt))

	return messages
}

func (s *ChatService) ChatStream(ctx context.Context, req ChatRequest, results []types.SearchResult, sources []SourceDoc, onToken func(token string) error) (*ChatResponse, error) {
	start := time.Now()

	messages := s.buildMessages(req, results)

	gen, err := s.newGeneratorFn(req)
	if err != nil {
		return nil, fmt.Errorf("create generator: %w", err)
	}

	completion, err := gen.GenerateStream(ctx, openai.ChatCompletionNewParams{
		Messages:    messages,
		MaxTokens:   openai.Int(int64(req.MaxTokens)),
		Temperature: openai.Float(*req.Temperature),
	}, onToken)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	answer := completion.Choices[0].Message.Content
	if req.ConversationID != "" {
		s.memory.Add(req.ConversationID, req.Query, answer)
	}

	return &ChatResponse{
		Answer:          answer,
		SourceDocuments: sources,
		TokenUsage: TokenUsage{
			Prompt:     int(completion.Usage.PromptTokens),
			Completion: int(completion.Usage.CompletionTokens),
			Total:      int(completion.Usage.TotalTokens),
		},
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

func (s *ChatService) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	start := time.Now()

	results, sources, err := s.retrieveSources(ctx, req)
	if err != nil {
		return nil, err
	}

	messages := s.buildMessages(req, results)

	gen, err := s.newGeneratorFn(req)
	if err != nil {
		return nil, fmt.Errorf("create generator: %w", err)
	}

	completion, err := gen.Generate(ctx, openai.ChatCompletionNewParams{
		Messages:    messages,
		MaxTokens:   openai.Int(int64(req.MaxTokens)),
		Temperature: openai.Float(*req.Temperature),
	})
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	answer := completion.Choices[0].Message.Content
	if req.ConversationID != "" {
		s.memory.Add(req.ConversationID, req.Query, answer)
	}

	return &ChatResponse{
		Answer:          answer,
		SourceDocuments: sources,
		TokenUsage: TokenUsage{
			Prompt:     int(completion.Usage.PromptTokens),
			Completion: int(completion.Usage.CompletionTokens),
			Total:      int(completion.Usage.TotalTokens),
		},
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}
