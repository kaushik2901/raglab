package workflow

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestCloneArgs_Kind(t *testing.T) {
	assert.Equal(t, "clone", CloneArgs{}.Kind())
}

func TestPreprocessArgs_Kind(t *testing.T) {
	assert.Equal(t, "preprocess", PreprocessArgs{}.Kind())
}

func TestVerifyArgs_Kind(t *testing.T) {
	assert.Equal(t, "verify", VerifyArgs{}.Kind())
}

func TestCloneWorker_Work_ErrorPropagation(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("returns error when workflow does not exist", func(t *testing.T) {
		cleanTables(t, pool)
		w := NewCloneWorker(store, nil)
		job := &river.Job[CloneArgs]{
			Args: CloneArgs{
				WorkflowID: "00000000-0000-0000-0000-000000000000",
				RepoURL:    "https://example.com/repo.git",
				RepoPath:   "/tmp/test-repo",
				Tag:        "test-clone-err",
			},
		}
		err := w.Work(context.Background(), job)
		require.Error(t, err)
	})
}

func TestCloneWorker_Work_StageExecution(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("creates step and fails on invalid git URL", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, err := store.CreateWorkflow(context.Background(), "preprocess", "test-clone-exec", map[string]any{
			"repo_url": "https://invalid-repo-url-12345.com/repo.git",
		})
		require.NoError(t, err)

		w := NewCloneWorker(store, nil)
		job := &river.Job[CloneArgs]{
			Args: CloneArgs{
				WorkflowID: wfID,
				Tag:        "test-clone-exec",
				RepoURL:    "https://invalid-repo-url-12345.com/repo.git",
				RepoPath:   "/tmp/test-clone-exec-repo",
			},
		}

		err = w.Work(context.Background(), job)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "clone")

		steps, err := store.GetSteps(context.Background(), wfID)
		require.NoError(t, err)
		require.Len(t, steps, 1, "expected a step to be created")
		assert.Equal(t, "clone", steps[0].StepName)
		assert.Equal(t, "failed", steps[0].Status)

		wf, err := store.GetWorkflow(context.Background(), wfID)
		require.NoError(t, err)
		assert.NotEqual(t, "succeeded", wf.Status, "workflow should not be succeeded after failure")
	})
}

func TestPreprocessWorker_Work_ErrorPropagation(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("returns error when workflow does not exist", func(t *testing.T) {
		cleanTables(t, pool)
		w := NewPreprocessWorker(store, nil)
		job := &river.Job[PreprocessArgs]{
			Args: PreprocessArgs{
				WorkflowID: "00000000-0000-0000-0000-000000000000",
				RepoPath:   "/tmp/nonexistent",
				OutputPath: "/tmp/output",
				Tag:        "test-pp-err",
			},
		}
		err := w.Work(context.Background(), job)
		require.Error(t, err)
	})
}

func TestVerifyWorker_Work_ErrorPropagation(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("returns error when workflow does not exist", func(t *testing.T) {
		cleanTables(t, pool)
		w := NewVerifyWorker(store, nil)
		job := &river.Job[VerifyArgs]{
			Args: VerifyArgs{
				WorkflowID: "00000000-0000-0000-0000-000000000000",
				RepoPath:   "/tmp/nonexistent",
				OutputPath: "/tmp/output",
				Tag:        "test-verify-err",
			},
		}
		err := w.Work(context.Background(), job)
		require.Error(t, err)
	})
}

func TestWorker_UpdatesWorkflowOnSuccess(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("verify worker succeeds workflow on success", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, err := store.CreateWorkflow(context.Background(), "preprocess", "test-verify-ok", map[string]any{
			"repo_path": t.TempDir(),
		})
		require.NoError(t, err)

		stageCallback := func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			return &types.StageResult{
				Name:   "verify",
				Output: map[string]any{"passed": true},
			}, nil
		}

		err = store.runStep(context.Background(), wfID, "verify", stageCallback)
		require.NoError(t, err)

		err = store.UpdateWorkflowStatus(context.Background(), wfID, "succeeded")
		require.NoError(t, err)

		wf, err := store.GetWorkflow(context.Background(), wfID)
		require.NoError(t, err)
		assert.Equal(t, "succeeded", wf.Status)
	})

	t.Run("runStep propagates stage errors", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, err := store.CreateWorkflow(context.Background(), "preprocess", "test-verify-fail", nil)
		require.NoError(t, err)

		stageCallback := func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
			return nil, assert.AnError
		}

		err = store.runStep(context.Background(), wfID, "verify", stageCallback)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)

		steps, err := store.GetSteps(context.Background(), wfID)
		require.NoError(t, err)
		require.Len(t, steps, 1)
		assert.Equal(t, "failed", steps[0].Status)
	})
}
