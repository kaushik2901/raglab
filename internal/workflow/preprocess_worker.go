package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"

	"github.com/jackc/pgx/v5"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/stage"
	"github.com/riverqueue/river"
)

type PreprocessArgs struct {
	Tag         string   `json:"tag"`
	RepoURL     string   `json:"repo_url"`
	IncludeDirs []string `json:"include_dirs,omitempty"`
}

func (PreprocessArgs) Kind() string { return "preprocess" }

type PreprocessWorker struct {
	river.WorkerDefaults[PreprocessArgs]
	Client *river.Client[pgx.Tx]
}

func (w *PreprocessWorker) Work(ctx context.Context, job *river.Job[PreprocessArgs]) error {
	logger := slog.With("job_id", job.ID, "worker", "preprocess")
	logger.Debug("starting preprocess workflow")

	args := job.Args
	repoPath := path.Join("artifacts", "preprocessing", args.Tag, "repo")
	outputPath := path.Join("artifacts", "preprocessing", args.Tag, "output")

	state := map[string]any{"repo_path": repoPath}

	checkpoint := readCheckpoint(job)

	// Step 1: Clone
	if !checkpoint["clone_done"] {
		logger.Debug("running clone step")
		if _, err := stage.CloneStage(args.RepoURL, repoPath).Run(ctx, state); err != nil {
			return fmt.Errorf("clone: %w", err)
		}
		if err := w.saveCheckpoint(ctx, job, "clone_done", checkpoint); err != nil {
			return fmt.Errorf("save checkpoint after clone: %w", err)
		}
		checkpoint["clone_done"] = true
		logger.Debug("clone step completed")
	}

	// Step 2: Preprocess
	if !checkpoint["preprocess_done"] {
		logger.Debug("running preprocess step")
		if _, err := stage.PreprocessStage(outputPath, args.IncludeDirs).Run(ctx, state); err != nil {
			return fmt.Errorf("preprocess: %w", err)
		}
		if err := w.saveCheckpoint(ctx, job, "preprocess_done", checkpoint); err != nil {
			return fmt.Errorf("save checkpoint after preprocess: %w", err)
		}
		checkpoint["preprocess_done"] = true
		logger.Debug("preprocess step completed")
	}

	// Step 3: Verify
	logger.Debug("running verify step")
	if _, err := stage.VerifyStage(outputPath, args.IncludeDirs).Run(ctx, state); err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	logger.Info("preprocess workflow complete", "tag", args.Tag)
	return nil
}

func readCheckpoint(job *river.Job[PreprocessArgs]) map[string]bool {
	cp := map[string]bool{}
	raw := job.Output()
	if len(raw) == 0 {
		return cp
	}
	var data map[string]bool
	if err := json.Unmarshal(raw, &data); err != nil {
		return cp
	}
	for k, v := range data {
		cp[k] = v
	}
	return cp
}

func (w *PreprocessWorker) saveCheckpoint(ctx context.Context, job *river.Job[PreprocessArgs], step string, cp map[string]bool) error {
	cp[step] = true
	_, err := w.Client.JobUpdate(ctx, job.ID, &river.JobUpdateParams{
		Output: cp,
	})
	return err
}
