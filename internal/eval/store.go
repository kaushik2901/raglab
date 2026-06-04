package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type EvalStore struct {
	pool *pgxpool.Pool
}

func NewEvalStore(pool *pgxpool.Pool) *EvalStore {
	return &EvalStore{pool: pool}
}

func (s *EvalStore) CreateRun(ctx context.Context, workflowID, tag string, strategy map[string]any) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO eval_runs (workflow_id, tag, strategy) VALUES ($1, $2, $3) RETURNING id`,
		workflowID, tag, strategy,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create eval run: %w", err)
	}
	return id, nil
}

func (s *EvalStore) AddQueryResult(ctx context.Context, runID string, r types.RetrievalResult) error {
	hitJSON, err := json.Marshal(r.Hit)
	if err != nil {
		return fmt.Errorf("marshal hit: %w", err)
	}
	retrievedJSON, err := json.Marshal(r.RetrievedPaths)
	if err != nil {
		return fmt.Errorf("marshal retrieved: %w", err)
	}
	expectedPathsJSON, err := json.Marshal(r.ExpectedPaths)
	if err != nil {
		return fmt.Errorf("marshal expected paths: %w", err)
	}
	relevanceJSON, err := json.Marshal(r.Relevance)
	if err != nil {
		return fmt.Errorf("marshal relevance: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO eval_queries (run_id, question_id, question, generated_answer, expected_paths, retrieved, relevance, hit, rank_first, prompt_tokens, completion_tokens, latency_ms, answer_score)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		runID, r.QuestionID, r.Question, r.Answer, expectedPathsJSON, retrievedJSON, relevanceJSON, hitJSON, r.RankFirst, r.PromptTokens, r.CompletionTokens, r.LatencyMs, r.AnswerScore,
	)
	if err != nil {
		return fmt.Errorf("insert query result: %w", err)
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

func (s *EvalStore) GetRunResults(ctx context.Context, runID string) ([]types.RetrievalResult, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT question_id, question, generated_answer, expected_paths, retrieved, relevance, hit, rank_first, prompt_tokens, completion_tokens, latency_ms, answer_score
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

		if err := rows.Scan(&r.QuestionID, &r.Question, &r.Answer, &expectedPathsJSON, &retrievedJSON, &relevanceJSON, &hitJSON, &r.RankFirst, &r.PromptTokens, &r.CompletionTokens, &r.LatencyMs, &r.AnswerScore); err != nil {
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
