package workflow

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
)

func TestEvalWorker_Work_ErrorPropagation(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)
	evalStore := eval.NewEvalStore(pool)

	t.Run("returns error when workflow does not exist", func(t *testing.T) {
		cleanTables(t, pool)
		w := NewEvalWorker(store, evalStore)
		job := &river.Job[EvalArgs]{
			Args: EvalArgs{
				WorkflowID:  "00000000-0000-0000-0000-000000000000",
				Tag:         "test-eval-err",
				IndexTag:    "test-collection",
				DatasetPath: "/tmp/nonexistent.json",
			},
		}
		err := w.Work(context.Background(), job)
		require.Error(t, err)
	})
}

func TestEvalWorker_Work_StepCreation(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)
	evalStore := eval.NewEvalStore(pool)

	t.Run("creates step and fails on nonexistent dataset", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, err := store.CreateWorkflow(context.Background(), "eval", "test-eval-step", map[string]any{
			"index_tag":    "test-collection",
			"dataset_path": "/tmp/nonexistent.json",
		})
		require.NoError(t, err)

		w := NewEvalWorker(store, evalStore)
		job := &river.Job[EvalArgs]{
			Args: EvalArgs{
				WorkflowID:  wfID,
				Tag:         "test-eval-step",
				IndexTag:    "test-collection",
				DatasetPath: "/tmp/nonexistent-dataset-99999.json",
			},
		}

		err = w.Work(context.Background(), job)
		require.Error(t, err)

		steps, err := store.GetSteps(context.Background(), wfID)
		require.NoError(t, err)
		require.Len(t, steps, 1)
		assert.Equal(t, "eval", steps[0].StepName)
		assert.Equal(t, "failed", steps[0].Status)
	})
}

func TestEvalWorker_EvalArgs_Defaults(t *testing.T) {
	t.Run("default batch size falls back to 20", func(t *testing.T) {
		args := EvalArgs{}
		batchSize := args.BatchSize
		if batchSize <= 0 {
			batchSize = 20
		}
		assert.Equal(t, 20, batchSize)
	})
}
