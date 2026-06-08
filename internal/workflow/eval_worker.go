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

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type EvalArgs struct {
	WorkflowID        string `json:"workflow_id"`
	Tag               string `json:"tag"`
	MainTag           string `json:"main_tag"`
	IndexTag          string `json:"index_tag"`
	QueryStrategy     string `json:"query_strategy"`
	DatasetPath       string `json:"dataset_path"`
	TopK              int    `json:"top_k"`
	LLMProvider       string `json:"llm_provider"`
	LLMModel          string `json:"llm_model"`
	EmbeddingProvider string `json:"embedding_provider"`
	EmbeddingModel    string `json:"embedding_model"`
	JudgeProvider     string `json:"judge_provider"`
	JudgeModel        string `json:"judge_model"`
	Concurrency       int    `json:"concurrency"`
	BatchSize         int    `json:"batch_size"`
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
	logger := slog.With("workflow_id", job.Args.WorkflowID, "worker", "eval")
	logger.Debug("starting eval worker")

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
		qdrantURL := os.Getenv("QDRANT_URL")
		if qdrantURL == "" {
			qdrantURL = "http://localhost:6334"
		}
		qdrantAPIKey := os.Getenv("QDRANT_API_KEY")

		evalRunID, err := w.EvalStore.CreateRun(ctx, args.WorkflowID, args.Tag, map[string]any{
			"index_tag":      args.IndexTag,
			"query_strategy": args.QueryStrategy,
			"top_k":          args.TopK,
			"llm_provider":   args.LLMProvider,
			"llm_model":      args.LLMModel,
			"judge_provider": args.JudgeProvider,
			"judge_model":    args.JudgeModel,
		})
		if err != nil {
			return nil, fmt.Errorf("create eval run: %w", err)
		}

		// 3. Connect to existing Qdrant collection
		llmProvider := config.Provider(args.LLMProvider)
		if llmProvider == "" {
			llmProvider = config.ProviderOpenAI
		}
		embeddingProvider := config.Provider(args.EmbeddingProvider)
		if embeddingProvider == "" {
			embeddingProvider = config.ProviderOpenAI
		}
		judgeProvider := config.Provider(args.JudgeProvider)
		if judgeProvider == "" {
			judgeProvider = llmProvider
		}

		embeddingModel := args.EmbeddingModel
		if embeddingModel == "" {
			embeddingModel = "text-embedding-3-small"
		}
		emb, err := embedder.New(embeddingProvider, embeddingModel, args.BatchSize)
		if err != nil {
			return nil, fmt.Errorf("create embedder: %w", err)
		}
		qStore := qstore.NewQdrantStore(qdrantAPIKey)
		if err := qStore.Connect(ctx, qdrantURL); err != nil {
			return nil, fmt.Errorf("connect qdrant: %w", err)
		}
		defer qStore.Close()

		gen, err := generator.New(llmProvider, args.LLMModel)
		if err != nil {
			return nil, fmt.Errorf("create generator: %w", err)
		}
		judgeGen, err := generator.New(judgeProvider, args.JudgeModel)
		if err != nil {
			return nil, fmt.Errorf("create judge generator: %w", err)
		}

		// 4. Run evaluation pipeline
		slog.Info("running evaluation pipeline", "collection", args.IndexTag, "questions", len(dataset.Questions))
		batchSize := args.BatchSize
		if batchSize <= 0 {
			batchSize = 20
		}
		results, err := eval.Evaluate(ctx, eval.PipelineArgs{
			Embedder:    emb,
			Searcher:    qStore,
			Generator:   gen,
			JudgeGen:    judgeGen,
			Collection:  args.IndexTag,
			Questions:   dataset.Questions,
			TopK:        args.TopK,
			EmbedBatch:  batchSize,
			Concurrency: args.Concurrency,
		})
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
		failedCount := 0
		var totalLatency int64
		var totalPrompt, totalCompletion int
		for _, r := range results {
			if r.Failed {
				failedCount++
			}
			totalLatency += r.LatencyMs
			totalPrompt += r.PromptTokens
			totalCompletion += r.CompletionTokens
		}
		avgLatency := float64(totalLatency) / float64(max(len(results), 1))
		avgPrompt := float64(totalPrompt) / float64(max(len(results), 1))
		avgCompletion := float64(totalCompletion) / float64(max(len(results), 1))

		aggregate.AvgLatencyMs = avgLatency
		aggregate.TotalLatencyMs = totalLatency
		aggregate.AvgPromptTokens = avgPrompt
		aggregate.AvgCompletionTokens = avgCompletion
		aggregate.TotalPromptTokens = totalPrompt
		aggregate.TotalCompletionTokens = totalCompletion

		report := &types.EvalReport{
			RunID: args.Tag,
			Strategy: types.EvalStrategyConfig{
				Tag:               args.Tag,
				IndexTag:          args.IndexTag,
				QueryStrategy:     args.QueryStrategy,
				TopK:              args.TopK,
				LLMProvider:       args.LLMProvider,
				LLMModel:          args.LLMModel,
				EmbeddingProvider: args.EmbeddingProvider,
				EmbeddingModel:    args.EmbeddingModel,
				JudgeProvider:     args.JudgeProvider,
				JudgeModel:        args.JudgeModel,
			},
			Questions:       len(results),
			QuestionsFailed: failedCount,
			Aggregate:       aggregate,
			PerQuestion:     results,
		}

		datasetFile := strings.TrimSuffix(filepath.Base(args.DatasetPath), ".json")
		reportPath := filepath.Join("artifacts", "evaluation-results", args.MainTag, datasetFile+".json")
		if err := eval.WriteJSONReport(report, reportPath); err != nil {
			slog.Warn("failed to write report", "path", reportPath, "err", err)
		}

		eval.PrintReport(report)

		return &types.StageResult{
			Name: "eval",
			Output: map[string]any{
				"eval_run_id":       evalRunID,
				"report_path":       reportPath,
				"question_count":    len(results),
				"questions_failed":  failedCount,
				"hit_rate@5":        aggregate.HitRate[5],
				"mrr":               aggregate.MRR,
				"avg_answer_score":  aggregate.AvgAnswerScore,
				"total_latency_ms":  totalLatency,
				"total_tokens":      totalPrompt + totalCompletion,
			},
		}, nil
	}); err != nil {
		return err
	}

	return w.Store.UpdateWorkflowStatus(ctx, args.WorkflowID, "succeeded")
}
