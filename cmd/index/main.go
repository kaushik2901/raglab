package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/workflow"
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

	cfg, err := config.Load()
	os.Args = origArgs
	if err != nil {
		return err
	}

	if cfg.InputTag == "" {
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

	cfg.Tag = resolveTag(cfg.Tag, "idx")

	wfID, err := store.CreateWorkflow(ctx, "index", cfg.Tag, map[string]any{
		"input_tag":   cfg.InputTag,
		"chunk_strategy": cfg.ChunkStrategy,
		"chunk_size":  cfg.ChunkSize,
		"chunk_overlap": cfg.ChunkOverlap,
		"embedding_model": cfg.EmbeddingModel,
		"batch_size":  cfg.BatchSize,
	})
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	slog.Info("created workflow", "id", wfID, "tag", cfg.Tag, "input_tag", cfg.InputTag)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	_, err = riverClient.Insert(ctx, &workflow.ParseArgs{
		WorkflowID: wfID,
		Tag:        cfg.Tag,
		InputTag:   cfg.InputTag,
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
