package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

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
	flag.CommandLine = flag.NewFlagSet("preprocess", flag.ExitOnError)

	repoURL := flag.String("repo-url", config.EnvOrDefault("REPO_URL", "https://gitlab.com/gitlab-com/content-sites/handbook.git"), "Repository URL to clone")
	tag := flag.String("tag", config.EnvOrDefault("TAG", ""), "Workflow tag (auto-generated if empty)")
	includeDirsRaw := flag.String("include-dirs", config.EnvOrDefault("INCLUDE_DIRS", ""), "Comma-separated subdirectories to process (empty = process all)")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	rc, err := db.NewRiverClient(ctx, cfg.MaxRetries+1)
	if err != nil {
		return err
	}
	defer rc.Pool.Close()

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

	result, err := rc.Client.Insert(ctx, &workflow.PreprocessWorkflowArgs{
		Tag:         resolvedTag,
		RepoURL:     *repoURL,
		IncludeDirs: includeDirs,
	}, &river.InsertOpts{
		Metadata: json.RawMessage("{}"),
	})
	if err != nil {
		return fmt.Errorf("insert preprocess job: %w", err)
	}

	jobID := result.Job.ID
	slog.Info("inserted preprocess job", "id", jobID, "tag", resolvedTag)

	row, err := workflow.PollUntilTerminal(ctx, rc.Client, jobID, 2*time.Second)
	if err != nil {
		return fmt.Errorf("preprocess job %d: %w", jobID, err)
	}
	if row.State != rivertype.JobStateCompleted {
		return fmt.Errorf("preprocess job %d failed: state=%s errors=%v", jobID, row.State, row.Errors)
	}

	slog.Info("preprocessing pipeline complete", "tag", resolvedTag)
	return nil
}
