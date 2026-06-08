package workflow

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"

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

func TestIndexArgs_DocTimeoutDefault(t *testing.T) {
	t.Run("zero timeout defaults to 30 minutes", func(t *testing.T) {
		args := IndexArgs{DocTimeout: 0}
		timeout := args.DocTimeout
		if timeout <= 0 {
			timeout = 30 * time.Minute
		}
		assert.Equal(t, 30*time.Minute, timeout)
	})

	t.Run("positive timeout is preserved", func(t *testing.T) {
		args := IndexArgs{DocTimeout: 10 * time.Minute}
		timeout := args.DocTimeout
		if timeout <= 0 {
			timeout = 30 * time.Minute
		}
		assert.Equal(t, 10*time.Minute, timeout)
	})
}

func TestMaxIndexFileSize_EnvVar(t *testing.T) {
	orig := os.Getenv("MAX_INDEX_FILE_SIZE")
	os.Setenv("MAX_INDEX_FILE_SIZE", "50")
	defer os.Setenv("MAX_INDEX_FILE_SIZE", orig)

	maxSize := config.IntEnvOrDefault("MAX_INDEX_FILE_SIZE", 100*1024*1024)
	assert.Equal(t, 50, maxSize, "should read from env var")
}

func TestMaxIndexFileSize_Default(t *testing.T) {
	orig := os.Getenv("MAX_INDEX_FILE_SIZE")
	os.Unsetenv("MAX_INDEX_FILE_SIZE")
	defer os.Setenv("MAX_INDEX_FILE_SIZE", orig)

	maxSize := config.IntEnvOrDefault("MAX_INDEX_FILE_SIZE", 100*1024*1024)
	assert.Equal(t, 100*1024*1024, maxSize, "should use default when env unset")
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
