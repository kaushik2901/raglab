package workflow

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	stagepkg "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/stage"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type CloneArgs struct {
	WorkflowID  string   `json:"workflow_id"`
	Tag         string   `json:"tag"`
	RepoURL     string   `json:"repo_url"`
	RepoPath    string   `json:"repo_path"`
	IncludeDirs []string `json:"include_dirs,omitempty"`
}

func (CloneArgs) Kind() string { return "clone" }

type PreprocessArgs struct {
	WorkflowID  string   `json:"workflow_id"`
	Tag         string   `json:"tag"`
	RepoPath    string   `json:"repo_path"`
	OutputPath  string   `json:"output_path"`
	IncludeDirs []string `json:"include_dirs,omitempty"`
}

func (PreprocessArgs) Kind() string { return "preprocess" }

type VerifyArgs struct {
	WorkflowID string `json:"workflow_id"`
	Tag        string `json:"tag"`
	RepoPath   string `json:"repo_path"`
	OutputPath string `json:"output_path"`
}

func (VerifyArgs) Kind() string { return "verify" }

type CloneWorker struct {
	river.WorkerDefaults[CloneArgs]
	Store  *Store
	Client *river.Client[pgx.Tx]
}

func NewCloneWorker(store *Store, client *river.Client[pgx.Tx]) *CloneWorker {
	return &CloneWorker{Store: store, Client: client}
}

func (w *CloneWorker) Work(ctx context.Context, job *river.Job[CloneArgs]) error {
	stepID, err := w.Store.CreateStep(ctx, job.Args.WorkflowID, "clone")
	if err != nil {
		return fmt.Errorf("create step: %w", err)
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "running", nil, nil); err != nil {
		return fmt.Errorf("mark step running: %w", err)
	}

	cfg := &config.Config{
		RepoURL:  job.Args.RepoURL,
		RepoPath: job.Args.RepoPath,
	}

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.CloneStage(cfg)
	result, err := stage.Run(ctx, state)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return err
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "succeeded", nil, result.Output); err != nil {
		return fmt.Errorf("mark step succeeded: %w", err)
	}

	repoPath := filepath.Join("artifacts", "preprocessing", job.Args.Tag, "repo")
	outPath := filepath.Join("artifacts", "preprocessing", job.Args.Tag, "output")
	_, err = w.Client.Insert(ctx, &PreprocessArgs{
		WorkflowID:  job.Args.WorkflowID,
		Tag:         job.Args.Tag,
		RepoPath:    repoPath,
		OutputPath:  outPath,
		IncludeDirs: job.Args.IncludeDirs,
	}, nil)
	return err
}

type PreprocessWorker struct {
	river.WorkerDefaults[PreprocessArgs]
	Store  *Store
	Client *river.Client[pgx.Tx]
}

func NewPreprocessWorker(store *Store, client *river.Client[pgx.Tx]) *PreprocessWorker {
	return &PreprocessWorker{Store: store, Client: client}
}

func (w *PreprocessWorker) Work(ctx context.Context, job *river.Job[PreprocessArgs]) error {
	stepID, err := w.Store.CreateStep(ctx, job.Args.WorkflowID, "preprocess")
	if err != nil {
		return fmt.Errorf("create step: %w", err)
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "running", nil, nil); err != nil {
		return fmt.Errorf("mark step running: %w", err)
	}

	cfg := &config.Config{
		RepoPath:    job.Args.RepoPath,
		OutputPath:  job.Args.OutputPath,
		IncludeDirs: job.Args.IncludeDirs,
	}

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.PreprocessStage(cfg)
	result, err := stage.Run(ctx, state)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return err
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "succeeded", nil, result.Output); err != nil {
		return fmt.Errorf("mark step succeeded: %w", err)
	}

	repoPath := filepath.Join("artifacts", "preprocessing", job.Args.Tag, "repo")
	outPath := filepath.Join("artifacts", "preprocessing", job.Args.Tag, "output")
	_, err = w.Client.Insert(ctx, &VerifyArgs{
		WorkflowID: job.Args.WorkflowID,
		Tag:        job.Args.Tag,
		RepoPath:   repoPath,
		OutputPath: outPath,
	}, nil)
	return err
}

type VerifyWorker struct {
	river.WorkerDefaults[VerifyArgs]
	Store  *Store
	Client *river.Client[pgx.Tx]
}

func NewVerifyWorker(store *Store, client *river.Client[pgx.Tx]) *VerifyWorker {
	return &VerifyWorker{Store: store, Client: client}
}

func (w *VerifyWorker) Work(ctx context.Context, job *river.Job[VerifyArgs]) error {
	stepID, err := w.Store.CreateStep(ctx, job.Args.WorkflowID, "verify")
	if err != nil {
		return fmt.Errorf("create step: %w", err)
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "running", nil, nil); err != nil {
		return fmt.Errorf("mark step running: %w", err)
	}

	cfg := &config.Config{
		RepoPath:   job.Args.RepoPath,
		OutputPath: job.Args.OutputPath,
	}

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.VerifyStage(cfg)
	result, err := stage.Run(ctx, state)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return err
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "succeeded", nil, result.Output); err != nil {
		return fmt.Errorf("mark step succeeded: %w", err)
	}

	if err := w.Store.UpdateWorkflowStatus(ctx, job.Args.WorkflowID, "succeeded"); err != nil {
		return fmt.Errorf("mark workflow succeeded: %w", err)
	}

	return nil
}

func RunCloneStep(ctx context.Context, args CloneArgs, state map[string]any) (*types.StageResult, error) {
	cfg := &config.Config{
		RepoURL:  args.RepoURL,
		RepoPath: args.RepoPath,
	}
	stage := stagepkg.CloneStage(cfg)
	return stage.Run(ctx, state)
}

func RunPreprocessStep(ctx context.Context, args PreprocessArgs, state map[string]any) (*types.StageResult, error) {
	cfg := &config.Config{
		RepoPath:    args.RepoPath,
		OutputPath:  args.OutputPath,
		IncludeDirs: args.IncludeDirs,
	}
	stage := stagepkg.PreprocessStage(cfg)
	return stage.Run(ctx, state)
}

func RunVerifyStep(ctx context.Context, args VerifyArgs, state map[string]any) (*types.StageResult, error) {
	cfg := &config.Config{
		RepoPath:   args.RepoPath,
		OutputPath: args.OutputPath,
	}
	stage := stagepkg.VerifyStage(cfg)
	return stage.Run(ctx, state)
}
