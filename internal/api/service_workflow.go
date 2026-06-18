package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/kaushik2901/raglab/internal/workflow"
)

func parseDocTimeout(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		slog.Warn("invalid doc_timeout value, using 0 (no timeout)", "value", s, "err", err)
		return 0
	}
	return d
}

type jobInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
	JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error)
	JobList(ctx context.Context, params *river.JobListParams) (*river.JobListResult, error)
}

type WorkflowService struct {
	client jobInserter
}

func NewWorkflowService(client jobInserter) *WorkflowService {
	return &WorkflowService{client: client}
}

func (s *WorkflowService) InsertPreprocess(ctx context.Context, req PreprocessRequest) (*WorkflowResponse, error) {
	result, err := s.client.Insert(ctx, &workflow.PreprocessArgs{
		Tag:         req.Tag,
		RepoURL:     req.RepoURL,
		BaseURL:     req.BaseURL,
		IncludeDirs: req.IncludeDirs,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("insert preprocess job: %w", err)
	}
	return jobToResponse(result.Job, req.Tag), nil
}

func (s *WorkflowService) InsertIndex(ctx context.Context, req IndexRequest) (*WorkflowResponse, error) {
	result, err := s.client.Insert(ctx, &workflow.IndexArgs{
		Tag:               req.Tag,
		InputTag:          req.InputTag,
		ParserStrategy:    req.ParserStrategy,
		ChunkStrategy:     req.ChunkStrategy,
		ChunkConfig:       req.ChunkConfig,
		EmbeddingProvider: req.EmbeddingProvider,
		EmbeddingModel:    req.EmbeddingModel,
		BatchSize:         req.BatchSize,
		IndexConcurrency:  req.IndexConcurrency,
		DocTimeout:        parseDocTimeout(req.DocTimeout),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("insert index job: %w", err)
	}
	return jobToResponse(result.Job, req.Tag), nil
}

func (s *WorkflowService) InsertEval(ctx context.Context, req EvalRequest) (*WorkflowResponse, error) {
	result, err := s.client.Insert(ctx, &workflow.EvalArgs{
		Tag:               req.Tag,
		IndexTag:          req.IndexTag,
		QueryStrategy:     req.QueryStrategy,
		DatasetPath:       req.DatasetPath,
		Ks:                req.Ks,
		LLMProvider:       req.LLMProvider,
		LLMModel:          req.LLMModel,
		EmbeddingProvider: req.EmbeddingProvider,
		EmbeddingModel:    req.EmbeddingModel,
		JudgeProvider:     req.JudgeProvider,
		JudgeModel:        req.JudgeModel,
		BatchSize:         req.BatchSize,
		Workers:           req.Workers,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("insert eval job: %w", err)
	}
	return jobToResponse(result.Job, req.Tag), nil
}

func (s *WorkflowService) ListJobs(ctx context.Context, kind, state string, limit, offset int) ([]JobEntry, int, error) {
	params := river.NewJobListParams()
	if kind != "" {
		params = params.Kinds(kind)
	}
	if state != "" {
		var states []rivertype.JobState
		for _, s := range strings.Split(state, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if ps := parseJobState(s); ps != "" {
				states = append(states, ps)
			}
		}
		if len(states) > 0 {
			params = params.States(states...)
		}
	}
	params = params.First(limit)

	result, err := s.client.JobList(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list jobs: %w", err)
	}

	jobs := make([]JobEntry, 0, len(result.Jobs))
	for _, j := range result.Jobs {
		entry := JobEntry{
			ID:          j.ID,
			Kind:        j.Kind,
			State:       jobStateString(j.State),
			Attempt:     j.Attempt,
			MaxAttempts: j.MaxAttempts,
			Tag:         extractTagFromJob(j),
		}
		if !j.CreatedAt.IsZero() {
			entry.CreatedAt = j.CreatedAt.Format(time.RFC3339)
		}
		if j.FinalizedAt != nil && !j.FinalizedAt.IsZero() {
			entry.FinalizedAt = j.FinalizedAt.Format(time.RFC3339)
		}
		jobs = append(jobs, entry)
	}

	return jobs, len(result.Jobs), nil
}

func (s *WorkflowService) GetJob(ctx context.Context, id int64) (*JobStatusResponse, error) {
	row, err := s.client.JobGet(ctx, id)
	if err != nil {
		return nil, err
	}

	resp := &JobStatusResponse{
		JobID: row.ID,
		Kind:  row.Kind,
		State: jobStateString(row.State),
	}
	if row.AttemptedAt != nil && !row.AttemptedAt.IsZero() {
		resp.AttemptedAt = row.AttemptedAt.Format(time.RFC3339)
	}
	if row.FinalizedAt != nil && !row.FinalizedAt.IsZero() {
		resp.CompletedAt = row.FinalizedAt.Format(time.RFC3339)
	}
	resp.Errors = formatErrors(row.Errors)
	return resp, nil
}

func jobToResponse(job *rivertype.JobRow, tag string) *WorkflowResponse {
	resp := &WorkflowResponse{
		JobID: job.ID,
		Tag:   tag,
		State: jobStateString(job.State),
	}
	if !job.CreatedAt.IsZero() {
		resp.CreatedAt = job.CreatedAt.Format(time.RFC3339)
	}
	return resp
}

func jobStateString(s rivertype.JobState) string {
	switch s {
	case rivertype.JobStateAvailable:
		return "available"
	case rivertype.JobStateRunning:
		return "running"
	case rivertype.JobStateRetryable:
		return "retrying"
	case rivertype.JobStateCompleted:
		return "completed"
	case rivertype.JobStateCancelled:
		return "cancelled"
	case rivertype.JobStateDiscarded:
		return "failed"
	default:
		return "unknown"
	}
}

func formatErrors(errs []rivertype.AttemptError) []string {
	if len(errs) == 0 {
		return nil
	}
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = fmt.Sprintf("attempt %d: %s", e.Attempt, e.Error)
	}
	return out
}

func extractTagFromJob(j *rivertype.JobRow) string {
	var args struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(j.EncodedArgs, &args); err != nil {
		return ""
	}
	return args.Tag
}

func parseJobState(s string) rivertype.JobState {
	switch s {
	case "available":
		return rivertype.JobStateAvailable
	case "running":
		return rivertype.JobStateRunning
	case "retrying":
		return rivertype.JobStateRetryable
	case "completed":
		return rivertype.JobStateCompleted
	case "cancelled":
		return rivertype.JobStateCancelled
	case "failed":
		return rivertype.JobStateDiscarded
	default:
		return ""
	}
}
