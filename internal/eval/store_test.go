package eval

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func connectOrSkip(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Connect(ctx)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres not reachable: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func runMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	require.NoError(t, db.Migrate(context.Background(), pool))
}

func cleanEvalTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `DELETE FROM eval_queries; DELETE FROM eval_runs;`)
	require.NoError(t, err)
}

func TestEvalStore_CreateRun(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	s := NewEvalStore(pool)

	t.Run("creates run with valid params", func(t *testing.T) {
		cleanEvalTables(t, pool)
		id, err := s.CreateRun(context.Background(), "wf-1", "eval-test-1", map[string]any{
			"index_tag": "idx-1",
			"top_k":     5,
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)
	})

	t.Run("creates multiple runs", func(t *testing.T) {
		cleanEvalTables(t, pool)
		id1, err := s.CreateRun(context.Background(), "wf-1", "eval-test-2", nil)
		require.NoError(t, err)
		id2, err := s.CreateRun(context.Background(), "wf-2", "eval-test-3", nil)
		require.NoError(t, err)
		assert.NotEqual(t, id1, id2)
	})
}

func TestEvalStore_AddQueryResult(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	s := NewEvalStore(pool)

	t.Run("inserts query result", func(t *testing.T) {
		cleanEvalTables(t, pool)
		runID, _ := s.CreateRun(context.Background(), "wf-1", "eval-add-q", nil)

		err := s.AddQueryResult(context.Background(), runID, types.RetrievalResult{
			QuestionID:       "q1",
			Question:         "test question?",
			Answer:           "test answer",
			ExpectedPaths:    []string{"doc1.md"},
			RetrievedPaths:   []string{"doc1.md", "doc2.md"},
			Scores:           []float64{0.9, 0.5},
			Hit:              map[int]bool{1: true, 3: true, 5: true},
			RankFirst:        1,
			PromptTokens:     10,
			CompletionTokens: 20,
			LatencyMs:        100,
			AnswerScore:      0.95,
		})
		require.NoError(t, err)
	})

	t.Run("fails for non-existent run", func(t *testing.T) {
		cleanEvalTables(t, pool)
		err := s.AddQueryResult(context.Background(), "00000000-0000-0000-0000-000000000000", types.RetrievalResult{
			QuestionID: "q1",
			Question:   "test?",
		})
		require.Error(t, err)
	})

	t.Run("inserts multiple results for same run", func(t *testing.T) {
		cleanEvalTables(t, pool)
		runID, _ := s.CreateRun(context.Background(), "wf-1", "eval-multi-q", nil)

		require.NoError(t, s.AddQueryResult(context.Background(), runID, types.RetrievalResult{
			QuestionID: "q1", Question: "q1?", Hit: map[int]bool{1: true},
		}))
		require.NoError(t, s.AddQueryResult(context.Background(), runID, types.RetrievalResult{
			QuestionID: "q2", Question: "q2?", Hit: map[int]bool{3: true},
		}))
	})

	t.Run("handles empty retrieved paths", func(t *testing.T) {
		cleanEvalTables(t, pool)
		runID, _ := s.CreateRun(context.Background(), "wf-1", "eval-empty-q", nil)

		err := s.AddQueryResult(context.Background(), runID, types.RetrievalResult{
			QuestionID: "q1",
			Question:   "test?",
			Hit:        map[int]bool{1: false, 3: false, 5: false},
		})
		require.NoError(t, err)
	})
}

func TestEvalStore_UpdateRunMetrics(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	s := NewEvalStore(pool)

	t.Run("updates metrics for existing run", func(t *testing.T) {
		cleanEvalTables(t, pool)
		runID, _ := s.CreateRun(context.Background(), "wf-1", "eval-metrics", nil)

		err := s.UpdateRunMetrics(context.Background(), runID, types.AggregateMetrics{
			HitRate:        map[int]float64{1: 0.5, 3: 0.8, 5: 0.9},
			MRR:            0.75,
			AvgAnswerScore: 0.85,
		})
		require.NoError(t, err)
	})

	t.Run("fails for non-existent run", func(t *testing.T) {
		cleanEvalTables(t, pool)
		err := s.UpdateRunMetrics(context.Background(), "00000000-0000-0000-0000-000000000000", types.AggregateMetrics{})
		require.Error(t, err)
	})
}

func TestEvalStore_GetRunResults(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	s := NewEvalStore(pool)

	t.Run("returns results in insertion order", func(t *testing.T) {
		cleanEvalTables(t, pool)
		runID, _ := s.CreateRun(context.Background(), "wf-1", "eval-get", nil)

		s.AddQueryResult(context.Background(), runID, types.RetrievalResult{
			QuestionID: "q1", Question: "first?", Hit: map[int]bool{1: true},
		})
		s.AddQueryResult(context.Background(), runID, types.RetrievalResult{
			QuestionID: "q2", Question: "second?", Hit: map[int]bool{3: false},
		})

		results, err := s.GetRunResults(context.Background(), runID)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "q1", results[0].QuestionID)
		assert.Equal(t, "q2", results[1].QuestionID)
	})

	t.Run("returns empty slice for run with no results", func(t *testing.T) {
		cleanEvalTables(t, pool)
		runID, _ := s.CreateRun(context.Background(), "wf-1", "eval-empty-get", nil)

		results, err := s.GetRunResults(context.Background(), runID)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("fails for non-existent run", func(t *testing.T) {
		cleanEvalTables(t, pool)
		_, err := s.GetRunResults(context.Background(), "00000000-0000-0000-0000-000000000000")
		require.Error(t, err)
	})
}

func TestEvalStore_FullRoundTrip(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	s := NewEvalStore(pool)

	t.Run("create, add results, update metrics, and retrieve", func(t *testing.T) {
		cleanEvalTables(t, pool)

		runID, err := s.CreateRun(context.Background(), "wf-roundtrip", "eval-roundtrip", map[string]any{
			"strategy": "naive-search",
			"top_k":    5,
		})
		require.NoError(t, err)

		results := []types.RetrievalResult{
			{
				QuestionID:       "q1",
				Question:         "what is x?",
				ExpectedAnswer:   "x is y (ground truth)",
				Answer:           "x is y",
				NDCGGraded:       0.85,
				ExpectedPaths:    []string{"doc1.md"},
				RetrievedPaths:   []string{"doc1.md", "doc2.md"},
				Scores:           []float64{0.95, 0.50},
				Hit:              map[int]bool{1: true, 3: true, 5: true},
				RankFirst:        1,
				Relevance:        []types.RelevanceJudgment{{DocumentPath: "doc1.md", Grade: 3}},
				PromptTokens:     10,
				CompletionTokens: 20,
				LatencyMs:        100,
				AnswerScore:      0.9,
			},
			{
				QuestionID:       "q2",
				Question:         "what is z?",
				ExpectedAnswer:   "z is w (ground truth)",
				Answer:           "z is w",
				NDCGGraded:       0.50,
				ExpectedPaths:    []string{"doc2.md"},
				RetrievedPaths:   []string{"doc3.md"},
				Scores:           []float64{0.30},
				Hit:              map[int]bool{1: false, 3: false, 5: true},
				RankFirst:        0,
				PromptTokens:     5,
				CompletionTokens: 10,
				LatencyMs:        50,
				AnswerScore:      0.5,
			},
		}

		for _, r := range results {
			require.NoError(t, s.AddQueryResult(context.Background(), runID, r))
		}

		metrics := types.AggregateMetrics{
			HitRate:        map[int]float64{1: 0.5, 3: 0.5, 5: 1.0},
			MRR:            0.5,
			NDCG:           map[int]float64{1: 0.5, 3: 0.5, 5: 0.75},
			Precision:      map[int]float64{1: 0.5, 3: 0.33, 5: 0.3},
			Recall:         map[int]float64{1: 0.25, 3: 0.25, 5: 0.5},
			AvgAnswerScore: 0.7,
		}
		require.NoError(t, s.UpdateRunMetrics(context.Background(), runID, metrics))

		got, err := s.GetRunResults(context.Background(), runID)
		require.NoError(t, err)
		require.Len(t, got, 2)

		assert.Equal(t, "q1", got[0].QuestionID)
		assert.Equal(t, "what is x?", got[0].Question)
		assert.Equal(t, "x is y (ground truth)", got[0].ExpectedAnswer)
		assert.Equal(t, "x is y", got[0].Answer)
		assert.InDelta(t, 0.85, got[0].NDCGGraded, 0.01)
		assert.Equal(t, []string{"doc1.md"}, got[0].ExpectedPaths)
		assert.Equal(t, []string{"doc1.md", "doc2.md"}, got[0].RetrievedPaths)
		assert.InDeltaSlice(t, []float64{0.95, 0.50}, got[0].Scores, 0.01)
		assert.Equal(t, map[int]bool{1: true, 3: true, 5: true}, got[0].Hit)
		assert.Equal(t, 1, got[0].RankFirst)
		assert.Equal(t, 10, got[0].PromptTokens)
		assert.Equal(t, 20, got[0].CompletionTokens)
		assert.Equal(t, int64(100), got[0].LatencyMs)
		assert.InDelta(t, 0.9, got[0].AnswerScore, 0.01)
	})
}

func TestEvalStore_ConcurrentWrites(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	s := NewEvalStore(pool)
	ctx := context.Background()

	t.Run("concurrent AddQueryResult calls", func(t *testing.T) {
		cleanEvalTables(t, pool)
		runID, _ := s.CreateRun(ctx, "wf-concurrent", "eval-concurrent", nil)

		done := make(chan error, 10)
		for i := 0; i < 10; i++ {
			go func(n int) {
				done <- s.AddQueryResult(ctx, runID, types.RetrievalResult{
					QuestionID: json.Number(string(rune('0' + n))).String(),
					Question:   "q",
					Hit:        map[int]bool{1: n == 0},
				})
			}(i)
		}

		for i := 0; i < 10; i++ {
			require.NoError(t, <-done)
		}

		results, err := s.GetRunResults(ctx, runID)
		require.NoError(t, err)
		assert.Len(t, results, 10)
	})
}
