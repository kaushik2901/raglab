package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
)

func TestPreprocessHandler_ValidRequest(t *testing.T) {
	body := `{"repo_url": "https://github.com/example/repo.git", "tag": "test-pre"}`
	req := httptest.NewRequest("POST", "/api/v1/workflows/preprocess", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{
		svc: &WorkflowService{
			client: &mockJobClient{
				insertFn: func(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
					return &rivertype.JobInsertResult{
						Job: &rivertype.JobRow{
							ID:    100,
							State: rivertype.JobStateAvailable,
						},
					}, nil
				},
			},
		},
	}
	r.preprocessHandler(rec, req)

	assert.Equal(t, 202, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "/api/v1/workflows/100", rec.Header().Get("Location"))

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, float64(100), data["job_id"])
	assert.Equal(t, "available", data["state"])
}

func TestPreprocessHandler_MissingRepoURL(t *testing.T) {
	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/workflows/preprocess", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{
		svc: &WorkflowService{
			client: &mockJobClient{
				insertFn: func(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
					return nil, fmt.Errorf("repo_url is required")
				},
			},
		},
	}
	r.preprocessHandler(rec, req)

	assert.Equal(t, 400, rec.Code)
	var p ProblemDetail
	json.NewDecoder(rec.Body).Decode(&p)
	assert.Equal(t, "Invalid Parameter", p.Title)
}

func TestPreprocessHandler_InvalidJSON(t *testing.T) {
	body := `{bad json`
	req := httptest.NewRequest("POST", "/api/v1/workflows/preprocess", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{}
	r.preprocessHandler(rec, req)

	assert.Equal(t, 400, rec.Code)
}

func TestIndexHandler_ValidRequest(t *testing.T) {
	body := `{"input_tag": "pre-tag-123", "tag": "idx-fixed"}`
	req := httptest.NewRequest("POST", "/api/v1/workflows/index", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{
		svc: &WorkflowService{
			client: &mockJobClient{
				insertFn: func(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
					return &rivertype.JobInsertResult{
						Job: &rivertype.JobRow{
							ID:    200,
							State: rivertype.JobStateAvailable,
						},
					}, nil
				},
			},
		},
	}
	r.indexHandler(rec, req)

	assert.Equal(t, 202, rec.Code)
}

func TestIndexHandler_MissingInputTag(t *testing.T) {
	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/workflows/index", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{}
	r.indexHandler(rec, req)

	assert.Equal(t, 400, rec.Code)
}

func TestEvalHandler_ValidRequest(t *testing.T) {
	body := `{"index_tag": "idx-fixed", "query_strategy": "naive-search", "dataset_path": "/data/dataset.json"}`
	req := httptest.NewRequest("POST", "/api/v1/workflows/eval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{
		svc: &WorkflowService{
			client: &mockJobClient{
				insertFn: func(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
					return &rivertype.JobInsertResult{
						Job: &rivertype.JobRow{
							ID:    300,
							State: rivertype.JobStateAvailable,
						},
					}, nil
				},
			},
		},
	}
	r.evalHandler(rec, req)

	assert.Equal(t, 202, rec.Code)
}

func TestEvalHandler_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing index_tag", `{"query_strategy": "naive", "dataset_path": "/d"}`},
		{"missing query_strategy", `{"index_tag": "idx", "dataset_path": "/d"}`},
		{"missing dataset_path", `{"index_tag": "idx", "query_strategy": "naive"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/workflows/eval", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			r := &WorkflowRouter{}
			r.evalHandler(rec, req)

			assert.Equal(t, 400, rec.Code)
		})
	}
}

func TestWorkflowStatusHandler_Found(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/workflows/42", nil)
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{
		svc: &WorkflowService{
			client: &mockJobClient{
				jobGetFn: func(ctx context.Context, id int64) (*rivertype.JobRow, error) {
					return &rivertype.JobRow{
						ID:    id,
						State: rivertype.JobStateCompleted,
					}, nil
				},
			},
		},
	}

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "42")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	r.workflowStatusHandler(rec, req)

	assert.Equal(t, 200, rec.Code)
}

func TestWorkflowStatusHandler_NotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/workflows/999", nil)
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{
		svc: &WorkflowService{
			client: &mockJobClient{
				jobGetFn: func(ctx context.Context, id int64) (*rivertype.JobRow, error) {
					return nil, river.ErrNotFound
				},
			},
		},
	}

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	r.workflowStatusHandler(rec, req)

	assert.Equal(t, 404, rec.Code)
}

func TestWorkflowStatusHandler_InvalidID(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/workflows/abc", nil)
	rec := httptest.NewRecorder()

	r := &WorkflowRouter{}

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	r.workflowStatusHandler(rec, req)

	assert.Equal(t, 400, rec.Code)
}

func TestPreprocessRequest_Validate(t *testing.T) {
	assert.NoError(t, PreprocessRequest{RepoURL: "https://example.com"}.Validate())
	assert.Error(t, PreprocessRequest{}.Validate())
}

func TestIndexRequest_Validate(t *testing.T) {
	assert.NoError(t, IndexRequest{InputTag: "pre-tag"}.Validate())
	assert.Error(t, IndexRequest{}.Validate())
}

func TestEvalRequest_Validate(t *testing.T) {
	assert.NoError(t, EvalRequest{
		IndexTag:      "idx",
		QueryStrategy: "naive",
		DatasetPath:   "/path",
	}.Validate())
	assert.Error(t, EvalRequest{}.Validate())
	assert.Error(t, EvalRequest{IndexTag: "idx"}.Validate())
	assert.Error(t, EvalRequest{IndexTag: "idx", QueryStrategy: "naive"}.Validate())
}

func TestChatRequest_Validate(t *testing.T) {
	assert.NoError(t, ChatRequest{Tag: "col", Query: "hello"}.Validate())
	assert.Error(t, ChatRequest{}.Validate())
	assert.Error(t, ChatRequest{Tag: "col"}.Validate())
	assert.Error(t, ChatRequest{Query: "hello"}.Validate())
}
