package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	os.Args = append([]string{"preprocess"}, os.Args[1:]...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	cfg, err := config.Load()
	os.Args = origArgs
	if err != nil {
		return err
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

	cfg.Tag = resolveTag(cfg.Tag, "pre")

	repoPath := filepath.Join("artifacts", "preprocessing", cfg.Tag, "repo")

	wfID, err := store.CreateWorkflow(ctx, "preprocess", cfg.Tag, map[string]any{
		"repo_url":     cfg.RepoURL,
		"include_dirs": cfg.IncludeDirs,
	})
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	slog.Info("created workflow", "id", wfID, "tag", cfg.Tag)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	_, err = riverClient.Insert(ctx, &workflow.CloneArgs{
		WorkflowID:  wfID,
		Tag:         cfg.Tag,
		RepoURL:     cfg.RepoURL,
		RepoPath:    repoPath,
		IncludeDirs: cfg.IncludeDirs,
	}, nil)
	if err != nil {
		return fmt.Errorf("insert clone job: %w", err)
	}

	slog.Info("inserted clone job, waiting for completion")
	return workflow.PollUntilDone(ctx, store, wfID, 2*time.Second)
}

func resolveTag(tag, prefix string) string {
	if tag != "" {
		return tag
	}
	return prefix + "-" + time.Now().Format("20060102-150405")
}
