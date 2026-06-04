package workflow

import (
	"context"
	"path"

	"github.com/jackc/pgx/v5"
	stagepkg "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/stage"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
	"github.com/riverqueue/river"
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
	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "clone", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		return stagepkg.CloneStage(job.Args.RepoURL, job.Args.RepoPath).Run(ctx, state)
	}); err != nil {
		return err
	}

	repoPath := path.Join("artifacts", "preprocessing", job.Args.Tag, "repo")
	outPath := path.Join("artifacts", "preprocessing", job.Args.Tag, "output")
	_, err := w.Client.Insert(ctx, &PreprocessArgs{
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
	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "preprocess", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		return stagepkg.PreprocessStage(job.Args.OutputPath, job.Args.IncludeDirs).Run(ctx, state)
	}); err != nil {
		return err
	}

	repoPath := path.Join("artifacts", "preprocessing", job.Args.Tag, "repo")
	outPath := path.Join("artifacts", "preprocessing", job.Args.Tag, "output")
	_, err := w.Client.Insert(ctx, &VerifyArgs{
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
	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "verify", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		return stagepkg.VerifyStage(job.Args.OutputPath).Run(ctx, state)
	}); err != nil {
		return err
	}

	return w.Store.UpdateWorkflowStatus(ctx, job.Args.WorkflowID, "succeeded")
}

