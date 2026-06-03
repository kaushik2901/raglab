package workflow

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
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

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.CloneStage(job.Args.RepoURL, job.Args.RepoPath)
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

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.PreprocessStage(job.Args.OutputPath, job.Args.IncludeDirs)
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

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.VerifyStage(job.Args.OutputPath)
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
	stage := stagepkg.CloneStage(args.RepoURL, args.RepoPath)
	return stage.Run(ctx, state)
}

func RunPreprocessStep(ctx context.Context, args PreprocessArgs, state map[string]any) (*types.StageResult, error) {
	stage := stagepkg.PreprocessStage(args.OutputPath, args.IncludeDirs)
	return stage.Run(ctx, state)
}

func RunVerifyStep(ctx context.Context, args VerifyArgs, state map[string]any) (*types.StageResult, error) {
	stage := stagepkg.VerifyStage(args.OutputPath)
	return stage.Run(ctx, state)
}
