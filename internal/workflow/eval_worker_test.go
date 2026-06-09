package workflow

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
)

func cleanEvalTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `DELETE FROM eval_queries; DELETE FROM eval_runs;`)
	require.NoError(t, err)
}

func TestEvalArgs_Kind(t *testing.T) {
	assert.Equal(t, "eval", EvalArgs{}.Kind())
}

func TestEvalWorker_Work_ErrorPropagation(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	evalStore := eval.NewEvalStore(pool)

	t.Run("returns error for nonexistent dataset", func(t *testing.T) {
		cleanEvalTables(t, pool)
		w := NewEvalWorker(evalStore)
		job := &river.Job[EvalArgs]{
			Args: EvalArgs{
				Tag:         "test-eval-err",
				IndexTag:    "test-collection",
				DatasetPath: "/tmp/nonexistent.json",
			},
		}
		err := w.Work(context.Background(), job)
		require.Error(t, err)
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
