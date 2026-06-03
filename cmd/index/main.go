package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
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
	origArgs := os.Args
	os.Args = append([]string{"index"}, os.Args[1:]...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	inputTag := flag.String("input-tag", envOrDefault("INPUT_TAG", ""), "Source preprocessed tag (for indexing)")
	chunkStrategy := flag.String("chunk-strategy", envOrDefault("CHUNK_STRATEGY", "fixed"), "Chunking strategy (fixed only)")
	chunkSize := flag.Int("chunk-size", intEnvOrDefault("CHUNK_SIZE", 512), "Target token count per chunk")
	chunkOverlap := flag.Int("chunk-overlap", intEnvOrDefault("CHUNK_OVERLAP", 64), "Token overlap between chunks")
	embeddingModel := flag.String("embedding-model", envOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"), "Embedding model name")
	batchSize := flag.Int("batch-size", intEnvOrDefault("BATCH_SIZE", 20), "Embedding batch size")
	tag := flag.String("tag", envOrDefault("TAG", ""), "Workflow tag (auto-generated if empty)")

	cfg, err := config.Load()
	os.Args = origArgs
	if err != nil {
		return err
	}
	_ = cfg

	if *inputTag == "" {
		return fmt.Errorf("--input-tag is required (preprocessed output tag to index)")
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

	resolvedTag := resolveTag(*tag, "idx")

	wfID, err := store.CreateWorkflow(ctx, "index", resolvedTag, map[string]any{
		"input_tag":       *inputTag,
		"chunk_strategy":  *chunkStrategy,
		"chunk_size":      *chunkSize,
		"chunk_overlap":   *chunkOverlap,
		"embedding_model": *embeddingModel,
		"batch_size":      *batchSize,
	})
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	slog.Info("created workflow", "id", wfID, "tag", resolvedTag, "input_tag", *inputTag)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	_, err = riverClient.Insert(ctx, &workflow.ParseArgs{
		WorkflowID: wfID,
		Tag:        resolvedTag,
		InputTag:   *inputTag,
	}, nil)
	if err != nil {
		return fmt.Errorf("insert parse job: %w", err)
	}

	slog.Info("inserted parse job, waiting for completion")
	return workflow.PollUntilDone(ctx, store, wfID, 2*time.Second)
}

func resolveTag(tag, prefix string) string {
	if tag != "" {
		return tag
	}
	return prefix + "-" + time.Now().Format("20060102-150405")
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func intEnvOrDefault(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}
