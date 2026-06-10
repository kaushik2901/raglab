package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/workflow"
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
		ChunkSize:         req.ChunkSize,
		ChunkOverlap:      req.ChunkOverlap,
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
