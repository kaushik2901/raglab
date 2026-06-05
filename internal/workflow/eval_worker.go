package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/riverqueue/river"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/retriever"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type EvalArgs struct {
	WorkflowID    string `json:"workflow_id"`
	Tag           string `json:"tag"`
	MainTag       string `json:"main_tag"`
	IndexTag      string `json:"index_tag"`
	QueryStrategy string `json:"query_strategy"`
	DatasetPath   string `json:"dataset_path"`
	TopK          int    `json:"top_k"`
	LLMModel      string `json:"llm_model"`
	JudgeModel    string `json:"judge_model"`
	Concurrency   int    `json:"concurrency"`
}

func (EvalArgs) Kind() string { return "eval" }

type EvalWorker struct {
	river.WorkerDefaults[EvalArgs]
	Store     *Store
	EvalStore *eval.EvalStore
}

func NewEvalWorker(store *Store, evalStore *eval.EvalStore) *EvalWorker {
	return &EvalWorker{Store: store, EvalStore: evalStore}
}

func (w *EvalWorker) Work(ctx context.Context, job *river.Job[EvalArgs]) error {
	args := job.Args
	ks := []int{1, 3, 5, 10}

	if err := w.Store.runStep(ctx, args.WorkflowID, "eval", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		// 1. Load ground truth
		data, err := os.ReadFile(args.DatasetPath)
		if err != nil {
			return nil, fmt.Errorf("read dataset: %w", err)
		}
		data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
		var dataset types.EvalDataset
		if err := json.Unmarshal(data, &dataset); err != nil {
			return nil, fmt.Errorf("parse dataset: %w", err)
		}
		slog.Info("loaded ground truth", "questions", len(dataset.Questions))

		// 2. Create eval run in DB
		llmAPIKey := os.Getenv("LLM_API_KEY")
		qdrantURL := os.Getenv("QDRANT_URL")
		if qdrantURL == "" {
			qdrantURL = "http://localhost:6334"
		}
		qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
		llmBaseURL := os.Getenv("LLM_BASE_URL")
		if llmBaseURL == "" {
			llmBaseURL = "https://api.openai.com/v1"
		}

		evalRunID, err := w.EvalStore.CreateRun(ctx, args.WorkflowID, args.Tag, map[string]any{
			"index_tag":      args.IndexTag,
			"query_strategy": args.QueryStrategy,
			"top_k":          args.TopK,
			"llm_model":      args.LLMModel,
			"judge_model":    args.JudgeModel,
		})
		if err != nil {
			return nil, fmt.Errorf("create eval run: %w", err)
		}

		// 3. Connect to existing Qdrant collection
		emb := embedder.New(llmBaseURL, llmAPIKey, "text-embedding-3-small", 1)
		qStore := qstore.NewQdrantStore(qdrantAPIKey)
		if err := qStore.Connect(ctx, qdrantURL); err != nil {
			return nil, fmt.Errorf("connect qdrant: %w", err)
		}
		defer qStore.Close()

		ret, err := retriever.New(emb, qStore, args.QueryStrategy)
		if err != nil {
			return nil, err
		}
		gen := generator.New(llmBaseURL, llmAPIKey, args.LLMModel)
		judgeGen := generator.New(llmBaseURL, llmAPIKey, args.JudgeModel)
		evaluator := eval.NewRetrievalEvaluatorWithJudge(ret, gen, judgeGen, args.TopK)

		// 4. Run retrieval evaluation against the existing collection
		slog.Info("running retrieval evaluation", "collection", args.IndexTag, "strategy", args.QueryStrategy, "concurrency", args.Concurrency)
		results, err := evaluator.Evaluate(ctx, args.IndexTag, dataset.Questions, args.Concurrency)
		if err != nil {
			return nil, fmt.Errorf("evaluation: %w", err)
		}

		// 5. Compute aggregate metrics
		aggregate := eval.ComputeAggregateMetrics(results, ks)
		slog.Info("evaluation complete",
			"questions", len(results),
			"hit_rate@5", fmt.Sprintf("%.3f", aggregate.HitRate[5]),
			"mrr", fmt.Sprintf("%.3f", aggregate.MRR),
			"avg_answer_score", fmt.Sprintf("%.3f", aggregate.AvgAnswerScore),
		)

		// 6. Persist results
		for _, r := range results {
			if err := w.EvalStore.AddQueryResult(ctx, evalRunID, r); err != nil {
				return nil, fmt.Errorf("store result for %s: %w", r.QuestionID, err)
			}
		}
		if err := w.EvalStore.UpdateRunMetrics(ctx, evalRunID, aggregate); err != nil {
			return nil, fmt.Errorf("update metrics: %w", err)
		}

		// 7. Write JSON report
		report := &types.EvalReport{
			RunID: args.Tag,
			Strategy: types.EvalStrategyConfig{
				Tag:           args.Tag,
				IndexTag:      args.IndexTag,
				QueryStrategy: args.QueryStrategy,
				TopK:          args.TopK,
				LLMModel:      args.LLMModel,
			},
			Questions:   len(results),
			Aggregate:   aggregate,
			PerQuestion: results,
		}

		datasetFile := strings.TrimSuffix(filepath.Base(args.DatasetPath), ".json")
		reportPath := filepath.Join("artifacts", "evaluation", args.MainTag, datasetFile+".json")
		if err := eval.WriteJSONReport(report, reportPath); err != nil {
			slog.Warn("failed to write report", "path", reportPath, "err", err)
		}

		eval.PrintReport(report)

		return &types.StageResult{
			Name: "eval",
			Output: map[string]any{
				"eval_run_id":      evalRunID,
				"report_path":      reportPath,
				"question_count":   len(results),
				"hit_rate@5":       aggregate.HitRate[5],
				"mrr":              aggregate.MRR,
				"avg_answer_score": aggregate.AvgAnswerScore,
			},
		}, nil
	}); err != nil {
		return err
	}

	return w.Store.UpdateWorkflowStatus(ctx, args.WorkflowID, "succeeded")
}
