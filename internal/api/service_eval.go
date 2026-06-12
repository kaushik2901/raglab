package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EvalService struct {
	pool *pgxpool.Pool
}

func NewEvalService(pool *pgxpool.Pool) *EvalService {
	return &EvalService{pool: pool}
}

func (s *EvalService) ListRuns(ctx context.Context, limit, offset int) ([]RunSummary, int, error) {
	var total int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM eval_runs`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count runs: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, tag, strategy, metrics, created_at,
			(SELECT COUNT(*) FROM eval_queries WHERE run_id = eval_runs.id) AS question_count
		FROM eval_runs ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query runs: %w", err)
	}
	defer rows.Close()

	var runs []RunSummary
	for rows.Next() {
		var r RunSummary
		var strategyJSON, metricsJSON []byte
		var createdAt time.Time
		if err := rows.Scan(&r.ID, &r.Tag, &strategyJSON, &metricsJSON, &createdAt, &r.QuestionCount); err != nil {
			return nil, 0, fmt.Errorf("scan run: %w", err)
		}
		if err := json.Unmarshal(strategyJSON, &r.Strategy); err != nil {
			r.Strategy = map[string]any{}
		}
		if metricsJSON != nil {
			json.Unmarshal(metricsJSON, &r.Metrics)
		}
		r.CreatedAt = createdAt.Format(time.RFC3339)
		runs = append(runs, r)
	}
	return runs, total, rows.Err()
}

func (s *EvalService) GetRunSummary(ctx context.Context, id string) (*RunSummary, error) {
	var r RunSummary
	var strategyJSON, metricsJSON []byte
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, tag, strategy, metrics, created_at
		FROM eval_runs WHERE id = $1`, id).Scan(&r.ID, &r.Tag, &strategyJSON, &metricsJSON, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("get run summary %s: %w", id, err)
	}
	if err := json.Unmarshal(strategyJSON, &r.Strategy); err != nil {
		r.Strategy = map[string]any{}
	}
	if metricsJSON != nil {
		json.Unmarshal(metricsJSON, &r.Metrics)
	}
	r.CreatedAt = createdAt.Format(time.RFC3339)
	return &r, nil
}

func (s *EvalService) GetRuns(ctx context.Context, ids []string) (map[string]RunSummary, error) {
	if len(ids) == 0 {
		return map[string]RunSummary{}, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, tag, strategy, metrics, created_at
		FROM eval_runs WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("query runs by ids: %w", err)
	}
	defer rows.Close()

	runs := make(map[string]RunSummary, len(ids))
	for rows.Next() {
		var r RunSummary
		var strategyJSON, metricsJSON []byte
		var createdAt time.Time
		if err := rows.Scan(&r.ID, &r.Tag, &strategyJSON, &metricsJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		if err := json.Unmarshal(strategyJSON, &r.Strategy); err != nil {
			r.Strategy = map[string]any{}
		}
		if metricsJSON != nil {
			json.Unmarshal(metricsJSON, &r.Metrics)
		}
		r.CreatedAt = createdAt.Format(time.RFC3339)
		runs[r.ID] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *EvalService) DeleteRun(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM eval_runs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete eval run %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("eval run %s: not found", id)
	}
	return nil
}

func (s *EvalService) GetRun(ctx context.Context, id string, limit, offset int) (*RunDetail, error) {
	var r RunDetail
	var strategyJSON, metricsJSON []byte
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, tag, strategy, metrics, created_at
		FROM eval_runs WHERE id = $1`, id).Scan(&r.ID, &r.Tag, &strategyJSON, &metricsJSON, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("get run %s: %w", id, err)
	}
	if err := json.Unmarshal(strategyJSON, &r.Strategy); err != nil {
		r.Strategy = map[string]any{}
	}
	if metricsJSON != nil {
		json.Unmarshal(metricsJSON, &r.Metrics)
	}
	r.CreatedAt = createdAt.Format(time.RFC3339)

	var total int
	err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM eval_queries WHERE run_id = $1`, id).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("count queries: %w", err)
	}
	r.Total = total
	r.QuestionCount = total

	rows, err := s.pool.Query(ctx, `
		SELECT question_id, question, category, difficulty, expected_answer, generated_answer,
			ndcg_graded, rank_first, answer_score, prompt_tokens, completion_tokens, latency_ms
		FROM eval_queries WHERE run_id = $1 ORDER BY created_at ASC LIMIT $2 OFFSET $3`, id, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query questions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		q := map[string]any{}
		var qid, question, cat, diff, expAns, genAns string
		var ndcg float64
		var rankFirst int
		var answerScore float64
		var promptTokens, completionTokens int
		var latencyMs int64
		if err := rows.Scan(&qid, &question, &cat, &diff, &expAns, &genAns,
			&ndcg, &rankFirst, &answerScore, &promptTokens, &completionTokens, &latencyMs); err != nil {
			return nil, fmt.Errorf("scan question: %w", err)
		}
		q["question_id"] = qid
		q["question"] = question
		q["category"] = cat
		q["difficulty"] = diff
		q["expected_answer"] = expAns
		q["generated_answer"] = genAns
		q["ndcg_graded"] = ndcg
		q["rank_first"] = rankFirst
		q["answer_score"] = answerScore
		q["prompt_tokens"] = promptTokens
		q["completion_tokens"] = completionTokens
		q["latency_ms"] = latencyMs
		r.Questions = append(r.Questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &r, nil
}
