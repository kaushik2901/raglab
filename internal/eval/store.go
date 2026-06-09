package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type EvalStore struct {
	pool *pgxpool.Pool
}

func NewEvalStore(pool *pgxpool.Pool) *EvalStore {
	return &EvalStore{pool: pool}
}

func (s *EvalStore) CreateRun(ctx context.Context, tag string, strategy map[string]any) (string, error) {
	if strategy == nil {
		strategy = make(map[string]any)
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO eval_runs (tag, strategy) VALUES ($1, $2) RETURNING id`,
		tag, strategy,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create eval run: %w", err)
	}
	return id, nil
}

var evalQueriesColumns = []string{
	"run_id", "question_id", "question", "category", "difficulty",
	"expected_answer", "generated_answer", "ndcg_graded",
	"expected_paths", "retrieved", "relevance", "hit",
	"rank_first", "prompt_tokens", "completion_tokens", "latency_ms", "answer_score",
}

func (s *EvalStore) BulkAddQueryResults(ctx context.Context, runID string, results []types.RetrievalResult) error {
	if len(results) == 0 {
		return nil
	}

	rows := make([][]any, len(results))
	for i, r := range results {
		hitJSON, err := json.Marshal(r.Hit)
		if err != nil {
			return fmt.Errorf("marshal hit for %s: %w", r.QuestionID, err)
		}
		retrievedJSON, err := json.Marshal(r.RetrievedPaths)
		if err != nil {
			return fmt.Errorf("marshal retrieved for %s: %w", r.QuestionID, err)
		}
		expectedPathsJSON, err := json.Marshal(r.ExpectedPaths)
		if err != nil {
			return fmt.Errorf("marshal expected paths for %s: %w", r.QuestionID, err)
		}
		relevanceJSON, err := json.Marshal(r.Relevance)
		if err != nil {
			return fmt.Errorf("marshal relevance for %s: %w", r.QuestionID, err)
		}
		rows[i] = []any{
			runID, r.QuestionID, r.Question, r.Category, r.Difficulty,
			r.ExpectedAnswer, r.Answer, r.NDCGGraded,
			expectedPathsJSON, retrievedJSON, relevanceJSON, hitJSON,
			r.RankFirst, r.PromptTokens, r.CompletionTokens, r.LatencyMs, r.AnswerScore,
		}
	}

	_, err := s.pool.CopyFrom(ctx, pgx.Identifier{"eval_queries"}, evalQueriesColumns, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("bulk insert query results: %w", err)
	}
	return nil
}

func (s *EvalStore) UpdateRunMetrics(ctx context.Context, runID string, metrics types.AggregateMetrics) error {
	metricsJSON, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE eval_runs SET metrics = $1 WHERE id = $2`,
		metricsJSON, runID,
	)
	if err != nil {
		return fmt.Errorf("update metrics: %w", err)
	}
	return nil
}

func (s *EvalStore) DeleteRunResults(ctx context.Context, runID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM eval_queries WHERE run_id = $1`, runID)
	if err != nil {
		return fmt.Errorf("delete run results: %w", err)
	}
	return nil
}

func (s *EvalStore) GetRunResults(ctx context.Context, runID string) ([]types.RetrievalResult, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT question_id, question, category, difficulty, expected_answer, generated_answer, ndcg_graded, expected_paths, retrieved, relevance, hit, rank_first, prompt_tokens, completion_tokens, latency_ms, answer_score
		 FROM eval_queries WHERE run_id = $1 ORDER BY created_at ASC`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("query results: %w", err)
	}
	defer rows.Close()

	var results []types.RetrievalResult
	for rows.Next() {
		var r types.RetrievalResult
		var hitJSON, retrievedJSON, expectedPathsJSON, relevanceJSON []byte

		if err := rows.Scan(&r.QuestionID, &r.Question, &r.Category, &r.Difficulty, &r.ExpectedAnswer, &r.Answer, &r.NDCGGraded, &expectedPathsJSON, &retrievedJSON, &relevanceJSON, &hitJSON, &r.RankFirst, &r.PromptTokens, &r.CompletionTokens, &r.LatencyMs, &r.AnswerScore); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		if err := json.Unmarshal(expectedPathsJSON, &r.ExpectedPaths); err != nil {
			return nil, fmt.Errorf("unmarshal expected paths: %w", err)
		}
		if err := json.Unmarshal(retrievedJSON, &r.RetrievedPaths); err != nil {
			return nil, fmt.Errorf("unmarshal retrieved: %w", err)
		}
		if err := json.Unmarshal(relevanceJSON, &r.Relevance); err != nil {
			return nil, fmt.Errorf("unmarshal relevance: %w", err)
		}
		if err := json.Unmarshal(hitJSON, &r.Hit); err != nil {
			return nil, fmt.Errorf("unmarshal hit: %w", err)
		}

		results = append(results, r)
	}
	return results, rows.Err()
}
