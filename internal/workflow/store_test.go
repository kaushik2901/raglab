package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func cleanTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `DELETE FROM workflow_steps; DELETE FROM workflows;`)
	require.NoError(t, err)
}

func TestStore_CreateAndGetWorkflow(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("creates with valid params", func(t *testing.T) {
		cleanTables(t, pool)
		id, err := store.CreateWorkflow(context.Background(), "preprocess", "test-pre-1", map[string]any{
			"repo_url": "https://example.com/repo.git",
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)

		wf, err := store.GetWorkflow(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, "preprocess", wf.Type)
		assert.Equal(t, "test-pre-1", wf.Tag)
		assert.Equal(t, "pending", wf.Status)
		assert.Equal(t, "https://example.com/repo.git", wf.InputParams["repo_url"])
		assert.False(t, wf.CreatedAt.IsZero())
		assert.False(t, wf.UpdatedAt.IsZero())
	})

	t.Run("rejects empty tag", func(t *testing.T) {
		cleanTables(t, pool)
		_, err := store.CreateWorkflow(context.Background(), "preprocess", "", map[string]any{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tag must not be empty")
	})
}

func TestStore_GetWorkflow_Errors(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("returns error for non-existent ID", func(t *testing.T) {
		_, err := store.GetWorkflow(context.Background(), "00000000-0000-0000-0000-000000000000")
		require.Error(t, err)
	})
}

func TestStore_UpdateWorkflowStatus(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("valid transition pending to running", func(t *testing.T) {
		cleanTables(t, pool)
		id, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-trans", nil)

		err := store.UpdateWorkflowStatus(context.Background(), id, "running")
		require.NoError(t, err)

		wf, _ := store.GetWorkflow(context.Background(), id)
		assert.Equal(t, "running", wf.Status)
	})

	t.Run("valid transition running to succeeded", func(t *testing.T) {
		cleanTables(t, pool)
		id, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-trans", nil)
		store.UpdateWorkflowStatus(context.Background(), id, "running")

		err := store.UpdateWorkflowStatus(context.Background(), id, "succeeded")
		require.NoError(t, err)

		wf, _ := store.GetWorkflow(context.Background(), id)
		assert.Equal(t, "succeeded", wf.Status)
	})

	t.Run("invalid transition succeeded to running", func(t *testing.T) {
		cleanTables(t, pool)
		id, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-trans", nil)
		store.UpdateWorkflowStatus(context.Background(), id, "running")
		store.UpdateWorkflowStatus(context.Background(), id, "succeeded")

		err := store.UpdateWorkflowStatus(context.Background(), id, "running")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid transition")
	})

	t.Run("invalid status string", func(t *testing.T) {
		cleanTables(t, pool)
		id, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-trans", nil)

		err := store.UpdateWorkflowStatus(context.Background(), id, "invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid workflow status")
	})

	t.Run("workflow not found", func(t *testing.T) {
		err := store.UpdateWorkflowStatus(context.Background(), "00000000-0000-0000-0000-000000000000", "running")
		require.Error(t, err)
	})
}

func TestStore_CreateAndUpdateStep(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("create step with valid workflow ID", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-step", nil)

		stepID, err := store.CreateStep(context.Background(), wfID, "clone")
		require.NoError(t, err)
		require.NotEmpty(t, stepID)
	})

	t.Run("fails for non-existent workflow ID", func(t *testing.T) {
		cleanTables(t, pool)
		_, err := store.CreateStep(context.Background(), "00000000-0000-0000-0000-000000000000", "clone")
		require.Error(t, err)
	})

	t.Run("update step status with error", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-step", nil)
		stepID, _ := store.CreateStep(context.Background(), wfID, "clone")

		err := store.UpdateStepStatus(context.Background(), stepID, "running", nil, nil)
		require.NoError(t, err)

		errStr := "something went wrong"
		err = store.UpdateStepStatus(context.Background(), stepID, "failed", &errStr, nil)
		require.NoError(t, err)

		steps, _ := store.GetSteps(context.Background(), wfID)
		require.Len(t, steps, 1)
		assert.Equal(t, "failed", steps[0].Status)
		require.NotNil(t, steps[0].Error)
		assert.Equal(t, "something went wrong", *steps[0].Error)
	})

	t.Run("update step status with output", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-step", nil)
		stepID, _ := store.CreateStep(context.Background(), wfID, "clone")

		store.UpdateStepStatus(context.Background(), stepID, "running", nil, nil)
		err := store.UpdateStepStatus(context.Background(), stepID, "succeeded", nil, map[string]any{
			"repo_path": "/tmp/repo",
			"commit":    "abc123",
		})
		require.NoError(t, err)

		steps, _ := store.GetSteps(context.Background(), wfID)
		require.Len(t, steps, 1)
		assert.Equal(t, "succeeded", steps[0].Status)
		assert.Equal(t, "/tmp/repo", steps[0].Output["repo_path"])
		assert.Equal(t, "abc123", steps[0].Output["commit"])
	})

	t.Run("invalid step transition", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-step", nil)
		stepID, _ := store.CreateStep(context.Background(), wfID, "clone")

		err := store.UpdateStepStatus(context.Background(), stepID, "succeeded", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid transition")
	})

	t.Run("step not found", func(t *testing.T) {
		err := store.UpdateStepStatus(context.Background(), "00000000-0000-0000-0000-000000000000", "running", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "step not found")
	})
}

func TestStore_GetSteps(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("returns steps in creation order", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-steps", nil)

		id1, _ := store.CreateStep(context.Background(), wfID, "clone")
		id2, _ := store.CreateStep(context.Background(), wfID, "preprocess")
		id3, _ := store.CreateStep(context.Background(), wfID, "verify")

		steps, err := store.GetSteps(context.Background(), wfID)
		require.NoError(t, err)
		require.Len(t, steps, 3)
		assert.Equal(t, id1, steps[0].ID)
		assert.Equal(t, id2, steps[1].ID)
		assert.Equal(t, id3, steps[2].ID)
	})

	t.Run("empty slice for no steps", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-steps", nil)

		steps, err := store.GetSteps(context.Background(), wfID)
		require.NoError(t, err)
		assert.Empty(t, steps)
	})
}

func TestStore_ListWorkflows(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("filters by type", func(t *testing.T) {
		cleanTables(t, pool)
		store.CreateWorkflow(context.Background(), "preprocess", "pre-1", nil)
		store.CreateWorkflow(context.Background(), "preprocess", "pre-2", nil)
		store.CreateWorkflow(context.Background(), "index", "idx-1", nil)

		result, err := store.ListWorkflows(context.Background(), "preprocess", "", "", 10, 0)
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("filters by tag", func(t *testing.T) {
		cleanTables(t, pool)
		store.CreateWorkflow(context.Background(), "preprocess", "pre-v1", nil)
		store.CreateWorkflow(context.Background(), "preprocess", "pre-v2", nil)

		result, err := store.ListWorkflows(context.Background(), "", "pre-v1", "", 10, 0)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "pre-v1", result[0].Tag)
	})

	t.Run("filters by status", func(t *testing.T) {
		cleanTables(t, pool)
		id1, _ := store.CreateWorkflow(context.Background(), "preprocess", "pre-1", nil)
		store.CreateWorkflow(context.Background(), "preprocess", "pre-2", nil)
		store.UpdateWorkflowStatus(context.Background(), id1, "running")

		result, err := store.ListWorkflows(context.Background(), "", "", "running", 10, 0)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "pre-1", result[0].Tag)
	})

	t.Run("paginates with limit and offset", func(t *testing.T) {
		cleanTables(t, pool)
		for i := 0; i < 5; i++ {
			store.CreateWorkflow(context.Background(), "preprocess", fmt.Sprintf("pre-%d", i), nil)
		}

		result, err := store.ListWorkflows(context.Background(), "", "", "", 2, 0)
		require.NoError(t, err)
		assert.Len(t, result, 2)

		result2, err := store.ListWorkflows(context.Background(), "", "", "", 2, 2)
		require.NoError(t, err)
		assert.Len(t, result2, 2)

		result3, err := store.ListWorkflows(context.Background(), "", "", "", 2, 4)
		require.NoError(t, err)
		assert.Len(t, result3, 1)
	})
}

func TestStore_LoadState(t *testing.T) {
	pool := connectOrSkip(t)
	runMigrations(t, pool)
	store := NewStore(pool)

	t.Run("merges input params and completed step outputs", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-state", map[string]any{
			"repo_url": "https://example.com/repo.git",
		})

		step1ID, _ := store.CreateStep(context.Background(), wfID, "clone")
		store.UpdateStepStatus(context.Background(), step1ID, "running", nil, nil)
		store.UpdateStepStatus(context.Background(), step1ID, "succeeded", nil, map[string]any{
			"repo_path": "/tmp/repo",
		})

		step2ID, _ := store.CreateStep(context.Background(), wfID, "preprocess")
		store.UpdateStepStatus(context.Background(), step2ID, "running", nil, nil)
		store.UpdateStepStatus(context.Background(), step2ID, "failed", strPtr("oops"), nil)

		state, err := store.LoadState(context.Background(), wfID)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/repo.git", state["repo_url"])
		assert.Equal(t, "/tmp/repo", state["repo_path"])
	})

	t.Run("empty when no completed steps", func(t *testing.T) {
		cleanTables(t, pool)
		wfID, _ := store.CreateWorkflow(context.Background(), "preprocess", "test-state", nil)
		store.CreateStep(context.Background(), wfID, "clone")

		state, err := store.LoadState(context.Background(), wfID)
		require.NoError(t, err)
		assert.Empty(t, state)
	})
}

func strPtr(s string) *string { return &s }
