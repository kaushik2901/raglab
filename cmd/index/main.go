package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/workflow"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	flag.CommandLine = flag.NewFlagSet("index", flag.ExitOnError)

	inputTag := flag.String("input-tag", config.EnvOrDefault("INPUT_TAG", ""), "Source preprocessed tag (for indexing)")
	parserStrategy := flag.String("parser", config.EnvOrDefault("PARSER", "markdown"), "Parser strategy (markdown)")
	chunkStrategy := flag.String("chunk-strategy", config.EnvOrDefault("CHUNK_STRATEGY", "fixed"), "Chunking strategy (fixed, semantic, recursive)")
	chunkSize := flag.Int("chunk-size", config.IntEnvOrDefault("CHUNK_SIZE", 512), "Target token count per chunk")
	chunkOverlap := flag.Int("chunk-overlap", config.IntEnvOrDefault("CHUNK_OVERLAP", 64), "Token overlap between chunks")
	llmProvider := flag.String("llm-provider", config.EnvOrDefault("LLM_PROVIDER", "openai"), "LLM provider (openai, gemini, openrouter, lmstudio)")
	embeddingProvider := flag.String("embedding-provider", config.EnvOrDefault("EMBEDDING_PROVIDER", ""), "Embedding provider (defaults to --llm-provider if empty)")
	embeddingModel := flag.String("embedding-model", config.EnvOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"), "Embedding model name")
	batchSize := flag.Int("batch-size", config.IntEnvOrDefault("BATCH_SIZE", 20), "Embedding batch size")
	indexConcurrency := flag.Int("index-concurrency", config.IntEnvOrDefault("INDEX_CONCURRENCY", 5), "Number of files to index concurrently")
	docTimeout := flag.Duration("doc-timeout", config.DurationEnvOrDefault("DOC_TIMEOUT", 30*time.Minute), "Timeout per document (parse+chunk+embed+store)")
	tag := flag.String("tag", config.EnvOrDefault("TAG", ""), "Workflow tag (auto-generated if empty)")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if *inputTag == "" {
		return fmt.Errorf("--input-tag is required (preprocessed output tag to index)")
	}

	if *embeddingProvider == "" {
		*embeddingProvider = *llmProvider
	}

	ctx := context.Background()

	pool, err := db.Connect(ctx)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("db migrate: %w", err)
	}

	store := workflow.NewStore(pool)

	resolvedTag := config.ResolveTag(*tag, "idx")

	wfID, err := store.CreateWorkflow(ctx, "index", resolvedTag, map[string]any{
		"input_tag":          *inputTag,
		"parser_strategy":    *parserStrategy,
		"chunk_strategy":     *chunkStrategy,
		"chunk_size":         *chunkSize,
		"chunk_overlap":      *chunkOverlap,
		"embedding_provider": *embeddingProvider,
		"embedding_model":    *embeddingModel,
		"batch_size":         *batchSize,
		"index_concurrency":  *indexConcurrency,
		"doc_timeout":        docTimeout.String(),
	})
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	slog.Info("created workflow", "id", wfID, "tag", resolvedTag, "input_tag", *inputTag)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		MaxAttempts: cfg.MaxRetries + 1,
	})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	_, err = riverClient.Insert(ctx, &workflow.IndexArgs{
		WorkflowID:        wfID,
		Tag:               resolvedTag,
		InputTag:          *inputTag,
		ParserStrategy:    *parserStrategy,
		ChunkStrategy:     *chunkStrategy,
		ChunkSize:         *chunkSize,
		ChunkOverlap:      *chunkOverlap,
		EmbeddingProvider: *embeddingProvider,
		EmbeddingModel:    *embeddingModel,
		BatchSize:         *batchSize,
		IndexConcurrency:  *indexConcurrency,
		DocTimeout:        *docTimeout,
	}, nil)
	if err != nil {
		return fmt.Errorf("insert parse job: %w", err)
	}

	slog.Info("inserted parse job, waiting for completion")
	if err := workflow.PollUntilDone(ctx, store, wfID, 2*time.Second); err != nil {
		return err
	}
	slog.Info("indexing pipeline complete", "tag", resolvedTag, "input_tag", *inputTag)
	return nil
}
