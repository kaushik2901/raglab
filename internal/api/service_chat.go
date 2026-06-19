package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go"

	"github.com/kaushik2901/raglab/internal/config"
	"github.com/kaushik2901/raglab/internal/embedder"
	"github.com/kaushik2901/raglab/internal/generator"
	"github.com/kaushik2901/raglab/internal/retriever"
	qstore "github.com/kaushik2901/raglab/internal/store"
	"github.com/kaushik2901/raglab/internal/types"
)

type retrieverInterface interface {
	Retrieve(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error)
}

type ChatService struct {
	vs   qstore.VectorStore
	cfg  *config.Config
	repo *ChatRepository

	newEmbedderFn  func(req ChatRequest) (embedder.Embedder, error)
	newRetrieverFn func() (retrieverInterface, error)
	newGeneratorFn func(req ChatRequest) (generator.Generator, error)
}

func NewChatService(cfg *config.Config, vs qstore.VectorStore, repo *ChatRepository) (*ChatService, error) {
	s := &ChatService{cfg: cfg, vs: vs, repo: repo}
	s.newEmbedderFn = func(req ChatRequest) (embedder.Embedder, error) {
		return embedder.New(config.Provider(req.EmbeddingProvider), req.EmbeddingModel, 1)
	}
	s.newRetrieverFn = func() (retrieverInterface, error) {
		return retriever.New(s.vs, retriever.StrategyNaiveSearch)
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
	ret, err := s.newRetrieverFn()
	if err != nil {
		return nil, nil, fmt.Errorf("create retriever: %w", err)
	}

	queryEmbeddings, err := emb.Embed(ctx, []types.Chunk{{ID: "query", Content: req.QueryText()}})
	if err != nil {
		return nil, nil, fmt.Errorf("embed query: %w", err)
	}
	if len(queryEmbeddings) == 0 {
		return nil, nil, fmt.Errorf("no embeddings returned")
	}
	queryVector := toFloat32(queryEmbeddings[0].Vector)

	results, err := ret.Retrieve(ctx, req.Tag, queryVector, req.TopK)
	if err != nil {
		return nil, nil, fmt.Errorf("retrieve: %w", err)
	}

	sources := make([]SourceDoc, len(results))
	for i, r := range results {
		sources[i] = SourceDoc{
			DocumentPath: r.DocumentPath,
			Score:        r.Score,
			SourceURL:    r.Metadata["source_url"],
		}
	}
	return results, sources, nil
}

func (s *ChatService) buildMessages(req ChatRequest, results []types.SearchResult) []openai.ChatCompletionMessageParamUnion {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a helpful assistant that answers questions based on the provided context. If the context does not contain enough information to answer, say so."),
	}

	if len(req.Messages) > 0 {
		history := req.Messages
		if len(history) > 0 && history[len(history)-1].Role == "user" {
			history = history[:len(history)-1]
		}
		for _, msg := range history {
			text := msg.TextContent()
			if text == "" {
				continue
			}
			switch msg.Role {
			case "user":
				messages = append(messages, openai.UserMessage(text))
			case "assistant":
				messages = append(messages, openai.AssistantMessage(text))
			}
		}
	}

	var contextParts []string
	for _, r := range results {
		label := r.DocumentPath
		if url := r.Metadata["source_url"]; url != "" {
			label = url
		}
		contextParts = append(contextParts, fmt.Sprintf("Document: %s\n%s", label, r.Content))
	}
	userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", strings.Join(contextParts, "\n\n"), req.QueryText())
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
		Temperature: openai.Float(req.Temperature),
	}, onToken)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	answer := completion.Choices[0].Message.Content

	convID, err := s.persistMessages(ctx, req, answer, sources, completion.Usage)
	if err != nil {
		return nil, fmt.Errorf("persist messages: %w", err)
	}

	return &ChatResponse{
		Answer:          answer,
		ConversationID:  convID,
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
		Temperature: openai.Float(req.Temperature),
	})
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	answer := completion.Choices[0].Message.Content

	convID, err := s.persistMessages(ctx, req, answer, sources, completion.Usage)
	if err != nil {
		return nil, fmt.Errorf("persist messages: %w", err)
	}

	return &ChatResponse{
		Answer:          answer,
		ConversationID:  convID,
		SourceDocuments: sources,
		TokenUsage: TokenUsage{
			Prompt:     int(completion.Usage.PromptTokens),
			Completion: int(completion.Usage.CompletionTokens),
			Total:      int(completion.Usage.TotalTokens),
		},
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

func (s *ChatService) persistMessages(ctx context.Context, req ChatRequest, answer string, sources []SourceDoc, usage openai.CompletionUsage) (string, error) {
	if s.repo == nil {
		return "", nil
	}

	convID, err := s.resolveConversationID(ctx, req)
	if err != nil {
		return "", err
	}

	sourcesJSON, _ := json.Marshal(sources)
	tokenUsageJSON, _ := json.Marshal(TokenUsage{
		Prompt:     int(usage.PromptTokens),
		Completion: int(usage.CompletionTokens),
		Total:      int(usage.TotalTokens),
	})

	query := req.QueryText()
	if err := s.repo.AddMessage(ctx, convID, "user", query, nil, nil); err != nil {
		return "", err
	}
	if err := s.repo.AddMessage(ctx, convID, "assistant", answer, sourcesJSON, tokenUsageJSON); err != nil {
		return "", err
	}
	return convID.String(), nil
}

func (s *ChatService) resolveConversationID(ctx context.Context, req ChatRequest) (uuid.UUID, error) {
	if req.ConversationID != "" {
		id, err := uuid.Parse(req.ConversationID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("invalid conversation_id: %w", err)
		}
		if err := s.repo.GetOrCreateConversation(ctx, id); err != nil {
			return uuid.Nil, err
		}
		return id, nil
	}
	id, err := s.repo.CreateConversation(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func toFloat32(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = float32(f)
	}
	return out
}
