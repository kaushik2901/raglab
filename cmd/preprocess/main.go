package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
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
	flag.CommandLine = flag.NewFlagSet("preprocess", flag.ExitOnError)

	repoURL := flag.String("repo-url", config.EnvOrDefault("REPO_URL", "https://gitlab.com/gitlab-com/content-sites/handbook.git"), "Repository URL to clone")
	tag := flag.String("tag", config.EnvOrDefault("TAG", ""), "Workflow tag (auto-generated if empty)")
	includeDirsRaw := flag.String("include-dirs", config.EnvOrDefault("INCLUDE_DIRS", ""), "Comma-separated subdirectories to process (empty = process all)")

	cfg, err := config.Load()
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

	resolvedTag := config.ResolveTag(*tag, "pre")

	var includeDirs []string
	if *includeDirsRaw != "" {
		for d := range strings.SplitSeq(*includeDirsRaw, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				includeDirs = append(includeDirs, d)
			}
		}
	}

	repoPath := path.Join("artifacts", "preprocessing", resolvedTag, "repo")

	wfID, err := store.CreateWorkflow(ctx, "preprocess", resolvedTag, map[string]any{
		"repo_url":     *repoURL,
		"include_dirs": includeDirs,
	})
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	slog.Info("created workflow", "id", wfID, "tag", resolvedTag)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		MaxAttempts: cfg.MaxRetries + 1,
	})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	_, err = riverClient.Insert(ctx, &workflow.CloneArgs{
		WorkflowID:  wfID,
		Tag:         resolvedTag,
		RepoURL:     *repoURL,
		RepoPath:    repoPath,
		IncludeDirs: includeDirs,
	}, nil)
	if err != nil {
		return fmt.Errorf("insert clone job: %w", err)
	}

	slog.Info("inserted clone job, waiting for completion")
	if err := workflow.PollUntilDone(ctx, store, wfID, 2*time.Second); err != nil {
		return err
	}
	slog.Info("preprocessing pipeline complete", "tag", resolvedTag)
	return nil
}
