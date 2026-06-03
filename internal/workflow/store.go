package workflow

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

var validWorkflowStatuses = map[string]bool{
	"pending": true, "running": true, "succeeded": true, "failed": true,
}

var validStepStatuses = map[string]bool{
	"pending": true, "running": true, "succeeded": true, "failed": true,
}

var workflowTransitions = map[string]map[string]bool{
	"pending":   {"running": true},
	"running":   {"succeeded": true, "failed": true},
	"failed":    {"running": true},
	"succeeded": {},
}

var stepTransitions = map[string]map[string]bool{
	"pending":   {"running": true},
	"running":   {"succeeded": true, "failed": true},
	"failed":    {"running": true},
	"succeeded": {},
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateWorkflow(ctx context.Context, wfType, tag string, params map[string]any) (string, error) {
	if tag == "" {
		return "", fmt.Errorf("tag must not be empty")
	}
	if params == nil {
		params = make(map[string]any)
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO workflows (type, tag, input_params) VALUES ($1, $2, $3) RETURNING id`,
		wfType, tag, params,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create workflow: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateWorkflowStatus(ctx context.Context, workflowID, status string) error {
	if !validWorkflowStatuses[status] {
		return fmt.Errorf("invalid workflow status: %s", status)
	}

	wf, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}

	if !workflowTransitions[wf.Status][status] {
		return fmt.Errorf("invalid transition from %s to %s", wf.Status, status)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE workflows SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, workflowID,
	)
	if err != nil {
		return fmt.Errorf("update workflow status: %w", err)
	}
	return nil
}

func (s *Store) GetWorkflow(ctx context.Context, workflowID string) (*types.Workflow, error) {
	var wf types.Workflow
	err := s.pool.QueryRow(ctx,
		`SELECT id, type, tag, status, input_params, created_at, updated_at FROM workflows WHERE id = $1`,
		workflowID,
	).Scan(&wf.ID, &wf.Type, &wf.Tag, &wf.Status, &wf.InputParams, &wf.CreatedAt, &wf.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	return &wf, nil
}

func (s *Store) ListWorkflows(ctx context.Context, wfType, tag, status string, limit, offset int) ([]types.Workflow, error) {
	where := ""
	args := make([]any, 0, 4)
	argIdx := 1

	if wfType != "" {
		where += fmt.Sprintf(" WHERE type = $%d", argIdx)
		args = append(args, wfType)
		argIdx++
	}
	if tag != "" {
		prefix := " WHERE"
		if where != "" {
			prefix = " AND"
		}
		where += fmt.Sprintf("%s tag = $%d", prefix, argIdx)
		args = append(args, tag)
		argIdx++
	}
	if status != "" {
		prefix := " WHERE"
		if where != "" {
			prefix = " AND"
		}
		where += fmt.Sprintf("%s status = $%d", prefix, argIdx)
		args = append(args, status)
		argIdx++
	}

	query := fmt.Sprintf(`SELECT id, type, tag, status, input_params, created_at, updated_at FROM workflows%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	defer rows.Close()

	var workflows []types.Workflow
	for rows.Next() {
		var wf types.Workflow
		if err := rows.Scan(&wf.ID, &wf.Type, &wf.Tag, &wf.Status, &wf.InputParams, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan workflow: %w", err)
		}
		workflows = append(workflows, wf)
	}
	return workflows, rows.Err()
}

func (s *Store) CreateStep(ctx context.Context, workflowID, stepName string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO workflow_steps (workflow_id, step_name) VALUES ($1, $2) RETURNING id`,
		workflowID, stepName,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create step: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateStepStatus(ctx context.Context, stepID, status string, errMsg *string, output map[string]any) error {
	if !validStepStatuses[status] {
		return fmt.Errorf("invalid step status: %s", status)
	}

	var currentStatus string
	err := s.pool.QueryRow(ctx, `SELECT status FROM workflow_steps WHERE id = $1`, stepID).Scan(&currentStatus)
	if err != nil {
		return fmt.Errorf("step not found: %w", err)
	}

	if !stepTransitions[currentStatus][status] {
		return fmt.Errorf("invalid transition from %s to %s", currentStatus, status)
	}

	if status == "running" {
		_, err = s.pool.Exec(ctx,
			`UPDATE workflow_steps SET status = $1, started_at = NOW(), attempts = attempts + 1 WHERE id = $2`,
			status, stepID,
		)
	} else {
		_, err = s.pool.Exec(ctx,
			`UPDATE workflow_steps SET status = $1, error = $2, output = $3, completed_at = NOW() WHERE id = $4`,
			status, errMsg, output, stepID,
		)
	}
	if err != nil {
		return fmt.Errorf("update step status: %w", err)
	}
	return nil
}

func (s *Store) GetSteps(ctx context.Context, workflowID string) ([]types.WorkflowStep, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workflow_id, step_name, status, attempts, error, output, started_at, completed_at, created_at
		 FROM workflow_steps WHERE workflow_id = $1 ORDER BY created_at ASC`,
		workflowID,
	)
	if err != nil {
		return nil, fmt.Errorf("get steps: %w", err)
	}
	defer rows.Close()

	var steps []types.WorkflowStep
	for rows.Next() {
		var step types.WorkflowStep
		if err := rows.Scan(&step.ID, &step.WorkflowID, &step.StepName, &step.Status, &step.Attempts, &step.Error, &step.Output, &step.StartedAt, &step.CompletedAt, &step.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func (s *Store) LoadState(ctx context.Context, workflowID string) (map[string]any, error) {
	wf, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	state := make(map[string]any)
	for k, v := range wf.InputParams {
		state[k] = v
	}

	steps, err := s.GetSteps(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	for _, step := range steps {
		if step.Status == "succeeded" && step.Output != nil {
			for k, v := range step.Output {
				state[k] = v
			}
		}
	}

	return state, nil
}
