package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/workflow"
)

type mockJobClient struct {
	insertFn func(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
	jobGetFn func(context.Context, int64) (*rivertype.JobRow, error)
}

func (m *mockJobClient) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return m.insertFn(ctx, args, opts)
}

func (m *mockJobClient) JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return m.jobGetFn(ctx, id)
}

func TestInsertPreprocess_Success(t *testing.T) {
	svc := &WorkflowService{
		client: &mockJobClient{
			insertFn: func(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
				pa, ok := args.(*workflow.PreprocessArgs)
				require.True(t, ok)
				assert.Equal(t, "https://example.com/repo.git", pa.RepoURL)
				assert.NotEmpty(t, pa.Tag)
				return &rivertype.JobInsertResult{
					Job: &rivertype.JobRow{
						ID:    42,
						State: rivertype.JobStateAvailable,
					},
				}, nil
			},
		},
	}

	resp, err := svc.InsertPreprocess(context.Background(), PreprocessRequest{
		RepoURL: "https://example.com/repo.git",
		Tag:     "my-tag",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(42), resp.JobID)
	assert.Equal(t, "my-tag", resp.Tag)
	assert.Equal(t, "available", resp.State)
}

func TestInsertPreprocess_EmptyURL(t *testing.T) {
	svc := &WorkflowService{
		client: &mockJobClient{
			insertFn: func(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
				return nil, errors.New("repo_url is required")
			},
		},
	}

	_, err := svc.InsertPreprocess(context.Background(), PreprocessRequest{
		RepoURL: "",
		Tag:     "test",
	})
	require.Error(t, err)
}

func TestInsertIndex_Success(t *testing.T) {
	svc := &WorkflowService{
		client: &mockJobClient{
			insertFn: func(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
				ia, ok := args.(*workflow.IndexArgs)
				require.True(t, ok)
				assert.Equal(t, "pre-tag", ia.InputTag)
				return &rivertype.JobInsertResult{
					Job: &rivertype.JobRow{
						ID:    99,
						State: rivertype.JobStateAvailable,
					},
				}, nil
			},
		},
	}

	resp, err := svc.InsertIndex(context.Background(), IndexRequest{
		InputTag: "pre-tag",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(99), resp.JobID)
	assert.Equal(t, "available", resp.State)
}

func TestInsertEval_Success(t *testing.T) {
	svc := &WorkflowService{
		client: &mockJobClient{
			insertFn: func(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
				ea, ok := args.(*workflow.EvalArgs)
				require.True(t, ok)
				assert.Equal(t, "idx-tag", ea.IndexTag)
				assert.Equal(t, "naive-search", ea.QueryStrategy)
				assert.Equal(t, "/path/to/dataset", ea.DatasetPath)
				return &rivertype.JobInsertResult{
					Job: &rivertype.JobRow{
						ID:    77,
						State: rivertype.JobStateAvailable,
					},
				}, nil
			},
		},
	}

	resp, err := svc.InsertEval(context.Background(), EvalRequest{
		IndexTag:      "idx-tag",
		QueryStrategy: "naive-search",
		DatasetPath:   "/path/to/dataset",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(77), resp.JobID)
}

func TestGetJob_Success(t *testing.T) {
	now := time.Now()
	svc := &WorkflowService{
		client: &mockJobClient{
			jobGetFn: func(ctx context.Context, id int64) (*rivertype.JobRow, error) {
				return &rivertype.JobRow{
					ID:          id,
					Kind:        "preprocess",
					State:       rivertype.JobStateCompleted,
					AttemptedAt: &now,
					FinalizedAt: &now,
				}, nil
			},
		},
	}

	resp, err := svc.GetJob(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, int64(42), resp.JobID)
	assert.Equal(t, "preprocess", resp.Kind)
	assert.Equal(t, "completed", resp.State)
	assert.NotEmpty(t, resp.AttemptedAt)
	assert.NotEmpty(t, resp.CompletedAt)
}

func TestGetJob_NotFound(t *testing.T) {
	svc := &WorkflowService{
		client: &mockJobClient{
			jobGetFn: func(ctx context.Context, id int64) (*rivertype.JobRow, error) {
				return nil, river.ErrNotFound
			},
		},
	}

	_, err := svc.GetJob(context.Background(), 999)
	assert.Error(t, err)
}

func TestGetJob_Running(t *testing.T) {
	svc := &WorkflowService{
		client: &mockJobClient{
			jobGetFn: func(ctx context.Context, id int64) (*rivertype.JobRow, error) {
				return &rivertype.JobRow{
					ID:    id,
					State: rivertype.JobStateRunning,
				}, nil
			},
		},
	}

	resp, err := svc.GetJob(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "running", resp.State)
	assert.Empty(t, resp.CompletedAt)
}

func TestGetJob_Failed(t *testing.T) {
	svc := &WorkflowService{
		client: &mockJobClient{
			jobGetFn: func(ctx context.Context, id int64) (*rivertype.JobRow, error) {
				return &rivertype.JobRow{
					ID:    id,
					State: rivertype.JobStateDiscarded,
					Errors: []rivertype.AttemptError{
						{Attempt: 1, Error: "something went wrong"},
					},
				}, nil
			},
		},
	}

	resp, err := svc.GetJob(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "failed", resp.State)
	assert.Len(t, resp.Errors, 1)
	assert.Contains(t, resp.Errors[0], "something went wrong")
}

func TestJobStateString(t *testing.T) {
	tests := []struct {
		state rivertype.JobState
		want  string
	}{
		{rivertype.JobStateAvailable, "available"},
		{rivertype.JobStateRunning, "running"},
		{rivertype.JobStateRetryable, "retrying"},
		{rivertype.JobStateCompleted, "completed"},
		{rivertype.JobStateCancelled, "cancelled"},
		{rivertype.JobStateDiscarded, "failed"},
		{rivertype.JobState("unknown"), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, jobStateString(tt.state))
	}
}
