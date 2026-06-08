package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/memory"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/retriever"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load .env: %w", err)
	}

	flag.CommandLine = flag.NewFlagSet("query", flag.ExitOnError)

	query := flag.String("query", "", "Question to answer (required)")
	tag := flag.String("tag", config.EnvOrDefault("TAG", ""), "Qdrant collection name (required)")
	queryStrategy := flag.String("query-strategy", retriever.StrategyNaiveSearch, "Query strategy (naive-search)")
	topK := flag.Int("top-k", 5, "Number of chunks to retrieve")
	llmProvider := flag.String("llm-provider", config.EnvOrDefault("LLM_PROVIDER", "openai"), "LLM provider (openai, gemini, openrouter, lmstudio)")
	embeddingProvider := flag.String("embedding-provider", config.EnvOrDefault("EMBEDDING_PROVIDER", ""), "Embedding provider (defaults to --llm-provider if empty)")
	embedModel := flag.String("embedding-model", config.EnvOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"), "Embedding model")
	llmModel := flag.String("llm-model", config.EnvOrDefault("LLM_MODEL", "gpt-4o-mini"), "LLM model for answer generation")
	temperature := flag.Float64("temperature", 0.3, "LLM temperature")
	maxTokens := flag.Int("max-tokens", 1024, "Max answer tokens")
	convID := flag.String("conversation-id", "", "Conversation ID for multi-turn memory")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if *query == "" {
		return fmt.Errorf("--query is required")
	}
	if *tag == "" {
		return fmt.Errorf("--tag is required")
	}

	if *embeddingProvider == "" {
		*embeddingProvider = *llmProvider
	}

	ctx := context.Background()

	emb, err := embedder.New(config.Provider(*embeddingProvider), *embedModel, 1)
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}
	qStore := qstore.NewQdrantStore(cfg.QdrantAPIKey)
	if err := qStore.Connect(ctx, cfg.QdrantURL); err != nil {
		return fmt.Errorf("connect qdrant: %w", err)
	}
	defer qStore.Close()

	ret, err := retriever.New(emb, qStore, *queryStrategy)
	if err != nil {
		return err
	}
	gen, err := generator.New(config.Provider(*llmProvider), *llmModel)
	if err != nil {
		return fmt.Errorf("create generator: %w", err)
	}
	mem := memory.NewRingBuffer(10)

	slog.Info("retrieving context", "collection", *tag, "strategy", *queryStrategy, "top_k", *topK)
	results, err := ret.Retrieve(ctx, *tag, *query, *topK)
	if err != nil {
		return fmt.Errorf("retrieve: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No relevant documents found.")
		return nil
	}

	fmt.Println("\nContext:")
	for _, r := range results {
		fmt.Printf("  - %s (score: %.2f)\n", r.DocumentPath, r.Score)
	}

	var messages []openai.ChatCompletionMessageParamUnion
	messages = append(messages, openai.SystemMessage("You are a helpful assistant that answers questions based solely on the provided context. If the context does not contain enough information to answer, say so."))

	if *convID != "" {
		for _, turn := range mem.Get(*convID) {
			messages = append(messages, openai.UserMessage(turn.User.Content))
			messages = append(messages, openai.AssistantMessage(turn.Assistant.Content))
		}
	}

	var contextParts []string
	for _, r := range results {
		contextParts = append(contextParts, fmt.Sprintf("Document: %s\nContent:\n%s", r.DocumentPath, r.Content))
	}
	contextText := strings.Join(contextParts, "\n\n---\n\n")
	userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s\n\nAnswer the question based on the context above.", contextText, *query)
	messages = append(messages, openai.UserMessage(userPrompt))

	slog.Info("generating answer", "model", *llmModel)
	completion, err := gen.Generate(ctx, openai.ChatCompletionNewParams{
		Messages:    messages,
		Temperature: openai.Float(*temperature),
		MaxTokens:   openai.Int(int64(*maxTokens)),
	})
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	if len(completion.Choices) == 0 {
		return fmt.Errorf("no response from LLM")
	}

	answer := completion.Choices[0].Message.Content
	fmt.Println("\nAnswer:")
	fmt.Println(answer)

	if *convID != "" {
		mem.Add(*convID, *query, answer)
	}

	usage := completion.Usage
	if usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
		fmt.Printf("\n---\nTokens: %d prompt + %d completion = %d total\n",
			usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	return nil
}
