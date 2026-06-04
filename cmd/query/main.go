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
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
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

	// Embed query
	slog.Info("embedding query")
	emb := embedder.New(*llmBaseURL, llmAPIKey, *embedModel, 1)
	queryChunk := types.Chunk{
		ID:      "query",
		Content: *query,
	}
	embeddings, err := emb.Embed(ctx, []types.Chunk{queryChunk})
	if err != nil {
		return fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 {
		return fmt.Errorf("no embeddings returned")
	}

	queryVector := make([]float32, len(embeddings[0].Vector))
	for i, v := range embeddings[0].Vector {
		queryVector[i] = float32(v)
	}

	// Search Qdrant
	slog.Info("searching Qdrant", "collection", *tag, "top_k", *topK)
	qStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := qStore.Connect(ctx, qdrantURL); err != nil {
		return fmt.Errorf("connect qdrant: %w", err)
	}
	defer qStore.Close()

	results, err := qStore.Search(ctx, *tag, queryVector, *topK)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No relevant documents found.")
		return nil
	}

	// Print context
	fmt.Println("\nContext:")
	for _, r := range results {
		fmt.Printf("  - %s (score: %.2f)\n", r.DocumentPath, r.Score)
	}

	// Build prompt
	var contextParts []string
	for _, r := range results {
		contextParts = append(contextParts, fmt.Sprintf("Document: %s\nContent:\n%s", r.DocumentPath, r.Content))
	}
	contextText := strings.Join(contextParts, "\n\n---\n\n")

	systemPrompt := `You are a helpful assistant that answers questions based solely on the provided context. If the context does not contain enough information to answer, say so.`
	userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s\n\nAnswer the question based on the context above.", contextText, *query)

	// Generate answer
	slog.Info("generating answer", "model", *llmModel)
	gen := generator.New(*llmBaseURL, llmAPIKey, *llmModel)
	completion, err := gen.Generate(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature: openai.Float(*temperature),
		MaxTokens:   openai.Int(int64(*maxTokens)),
	})
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	if len(completion.Choices) == 0 {
		return fmt.Errorf("no response from LLM")
	}

	fmt.Println("\nAnswer:")
	fmt.Println(completion.Choices[0].Message.Content)

	usage := completion.Usage
	if usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
		fmt.Printf("\n---\nTokens: %d prompt + %d completion = %d total\n",
			usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	return nil
}
