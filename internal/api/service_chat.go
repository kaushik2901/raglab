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
	embedder  embedder.Embedder
	retriever retrieverInterface
	generator generator.Generator
	memory    memory.Memory
}

func NewChatService(cfg *config.Config, vs qstore.VectorStore) (*ChatService, error) {
	llmProvider := config.Provider(config.EnvOrDefault("LLM_PROVIDER", "openai"))
	embeddingProvider := config.Provider(config.EnvOrDefault("EMBEDDING_PROVIDER", string(llmProvider)))
	llmModel := config.EnvOrDefault("LLM_MODEL", "gpt-4o-mini")
	embeddingModel := config.EnvOrDefault("EMBEDDING_MODEL", "text-embedding-3-small")

	emb, err := embedder.New(embeddingProvider, embeddingModel, 1)
	if err != nil {
		return nil, fmt.Errorf("create embedder: %w", err)
	}

	ret, err := retriever.New(emb, vs, "naive-search")
	if err != nil {
		return nil, fmt.Errorf("create retriever: %w", err)
	}

	gen, err := generator.New(llmProvider, llmModel)
	if err != nil {
		return nil, fmt.Errorf("create generator: %w", err)
	}

	mem := memory.NewRingBuffer(cfg.ChatMemorySize)
	return &ChatService{embedder: emb, retriever: ret, generator: gen, memory: mem}, nil
}

func (s *ChatService) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	start := time.Now()

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	results, err := s.retriever.Retrieve(ctx, req.Tag, req.Query, topK)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}

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

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	temperature := 0.3
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	completion, err := s.generator.Generate(ctx, openai.ChatCompletionNewParams{
		Messages:    messages,
		MaxTokens:   openai.Int(int64(maxTokens)),
		Temperature: openai.Float(temperature),
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

	sources := make([]SourceDoc, len(results))
	for i, r := range results {
		sources[i] = SourceDoc{DocumentPath: r.DocumentPath, Score: r.Score}
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
