package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	origArgs := os.Args
	os.Args = append([]string{"preprocess"}, os.Args[1:]...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	repoURL := flag.String("repo-url", envOrDefault("REPO_URL", "https://gitlab.com/gitlab-com/content-sites/handbook"), "Repository URL to clone")
	tag := flag.String("tag", envOrDefault("TAG", ""), "Workflow tag (auto-generated if empty)")
	includeDirsRaw := flag.String("include-dirs", envOrDefault("INCLUDE_DIRS", ""), "Comma-separated subdirectories to process (empty = process all)")

	cfg, err := config.Load()
	os.Args = origArgs
	if err != nil {
		return err
	}
	_ = cfg

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

	resolvedTag := resolveTag(*tag, "pre")

	var includeDirs []string
	if *includeDirsRaw != "" {
		for d := range strings.SplitSeq(*includeDirsRaw, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				includeDirs = append(includeDirs, d)
			}
		}
	}

	repoPath := filepath.Join("artifacts", "preprocessing", resolvedTag, "repo")

	wfID, err := store.CreateWorkflow(ctx, "preprocess", resolvedTag, map[string]any{
		"repo_url":     *repoURL,
		"include_dirs": includeDirs,
	})
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	slog.Info("created workflow", "id", wfID, "tag", resolvedTag)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
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
