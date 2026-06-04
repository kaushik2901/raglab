package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
	flag.CommandLine = flag.NewFlagSet("query", flag.ExitOnError)

	query := flag.String("query", "", "Question to answer (required)")
	tag := flag.String("tag", "", "Qdrant collection name (required)")
	topK := flag.Int("top-k", 5, "Number of chunks to retrieve")
	embedModel := flag.String("embedding-model", config.EnvOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"), "Embedding model")
	llmModel := flag.String("llm-model", config.EnvOrDefault("LLM_MODEL", "gpt-4o-mini"), "LLM model for answer generation")
	llmBaseURL := flag.String("llm-base-url", config.EnvOrDefault("LLM_BASE_URL", "https://api.openai.com"), "OpenAI-compatible API base URL")
	temperature := flag.Float64("temperature", 0.3, "LLM temperature")
	maxTokens := flag.Int("max-tokens", 1024, "Max answer tokens")
	convID := flag.String("conversation-id", "", "Conversation ID for multi-turn memory")
	flag.Parse()

	if *query == "" {
		return fmt.Errorf("--query is required")
	}
	if *tag == "" {
		return fmt.Errorf("--tag is required")
	}

	llmAPIKey := os.Getenv("LLM_API_KEY")
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")

	ctx := context.Background()

	emb := embedder.New(*llmBaseURL, llmAPIKey, *embedModel, 1)
	qStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := qStore.Connect(ctx, qdrantURL); err != nil {
		return fmt.Errorf("connect qdrant: %w", err)
	}
	defer qStore.Close()

	ret := retriever.New(emb, qStore)
	gen := generator.New(*llmBaseURL, llmAPIKey, *llmModel)
	mem := memory.NewRingBuffer(10)

	slog.Info("retrieving context", "collection", *tag, "top_k", *topK)
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
