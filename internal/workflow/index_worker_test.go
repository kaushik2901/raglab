package workflow

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestIndexArgs_Kind(t *testing.T) {
	assert.Equal(t, "index", IndexArgs{}.Kind())
}

func TestEvalArgs_Kind(t *testing.T) {
	assert.Equal(t, "eval", EvalArgs{}.Kind())
}

func TestRunIndexing_EmptyInput(t *testing.T) {
	t.Run("returns zero docs for nonexistent input dir (walk warning)", func(t *testing.T) {
		result, err := RunIndexing(context.Background(), IndexArgs{
			WorkflowID:        "test-wf",
			Tag:               "test-collection",
			InputTag:          "nonexistent-tag-12345",
			EmbeddingProvider: "openai",
			EmbeddingModel:    "text-embedding-3-small",
			BatchSize:         10,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, types.StageID("index"), result.Name)
		assert.Equal(t, int32(0), result.Output["document_count"])
	})
}

func TestIndexWorker_Work_ErrorPropagation(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("returns error when workflow does not exist", func(t *testing.T) {
		cleanTables(t, pool)
		w := NewIndexWorker(store, nil)
		job := &river.Job[IndexArgs]{
			Args: IndexArgs{
				WorkflowID: "00000000-0000-0000-0000-000000000000",
				Tag:        "test-collection",
				InputTag:   "test-input",
			},
		}
		err := w.Work(context.Background(), job)
		require.Error(t, err)
	})
}

func TestIndexWorker_Work_StageCreation(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("creates step and handles indexing failure gracefully", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, err := store.CreateWorkflow(context.Background(), "index", "test-idx-step", map[string]any{
			"input_tag": "nonexistent",
		})
		require.NoError(t, err)

		w := NewIndexWorker(store, nil)
		job := &river.Job[IndexArgs]{
			Args: IndexArgs{
				WorkflowID: wfID,
				Tag:        "test-collection-idx",
				InputTag:   "nonexistent-input-tag-99999",
			},
		}

		err = w.Work(context.Background(), job)
		require.Error(t, err)

		steps, err := store.GetSteps(context.Background(), wfID)
		require.NoError(t, err)
		require.Len(t, steps, 1)
		assert.Equal(t, "index", steps[0].StepName)
		assert.Equal(t, "failed", steps[0].Status)
	})
}
