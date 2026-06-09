package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/riverqueue/river"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type EvalArgs struct {
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
	EvalStore *eval.EvalStore
}

func NewEvalWorker(evalStore *eval.EvalStore) *EvalWorker {
	return &EvalWorker{EvalStore: evalStore}
}

func (w *EvalWorker) Work(ctx context.Context, job *river.Job[EvalArgs]) error {
	logger := slog.With("job_id", job.ID, "worker", "eval")
	logger.Debug("starting eval worker")

	args := job.Args
	ks := []int{1, 3, 5, 10}

	// 1. Load ground truth
	data, err := os.ReadFile(args.DatasetPath)
	if err != nil {
		return fmt.Errorf("read dataset: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	var dataset types.EvalDataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return fmt.Errorf("parse dataset: %w", err)
	}
	slog.Info("loaded ground truth", "questions", len(dataset.Questions))

	// 2. Create eval run in DB
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")

	evalRunID, err := w.EvalStore.CreateRun(ctx, args.Tag, map[string]any{
		"index_tag":      args.IndexTag,
		"query_strategy": args.QueryStrategy,
		"top_k":          args.TopK,
		"llm_provider":   args.LLMProvider,
		"llm_model":      args.LLMModel,
		"judge_provider": args.JudgeProvider,
		"judge_model":    args.JudgeModel,
	})
	if err != nil {
		return fmt.Errorf("create eval run: %w", err)
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
		return fmt.Errorf("create embedder: %w", err)
	}
	qStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := qStore.Connect(ctx, qdrantURL); err != nil {
		return fmt.Errorf("connect qdrant: %w", err)
	}
	defer qStore.Close()

	gen, err := generator.New(llmProvider, args.LLMModel)
	if err != nil {
		return fmt.Errorf("create generator: %w", err)
	}
	judgeGen, err := generator.New(judgeProvider, args.JudgeModel)
	if err != nil {
		return fmt.Errorf("create judge generator: %w", err)
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
		return fmt.Errorf("evaluation: %w", err)
	}

	// 5. Compute aggregate metrics
	aggregate := eval.ComputeAggregateMetrics(results, ks)
	slog.Info("evaluation complete",
		"questions", len(results),
		"hit_rate@5", fmt.Sprintf("%.3f", aggregate.HitRate[5]),
		"mrr", fmt.Sprintf("%.3f", aggregate.MRR),
		"avg_answer_score", fmt.Sprintf("%.3f", aggregate.AvgAnswerScore),
	)

	// 6. Persist results — compute latency/token aggregates first
	var totalLatency int64
	var totalPrompt, totalCompletion int
	for _, r := range results {
		totalLatency += r.LatencyMs
		totalPrompt += r.PromptTokens
		totalCompletion += r.CompletionTokens
	}
	aggregate.AvgLatencyMs = float64(totalLatency) / float64(max(len(results), 1))
	aggregate.TotalLatencyMs = totalLatency
	aggregate.AvgPromptTokens = float64(totalPrompt) / float64(max(len(results), 1))
	aggregate.AvgCompletionTokens = float64(totalCompletion) / float64(max(len(results), 1))
	aggregate.TotalPromptTokens = totalPrompt
	aggregate.TotalCompletionTokens = totalCompletion

	if err := w.EvalStore.BulkAddQueryResults(ctx, evalRunID, results); err != nil {
		return fmt.Errorf("store results: %w", err)
	}
	if err := w.EvalStore.UpdateRunMetrics(ctx, evalRunID, aggregate); err != nil {
		return fmt.Errorf("update metrics: %w", err)
	}

	logger.Info("evaluation complete",
		"questions", len(results),
		"hit_rate@5", fmt.Sprintf("%.3f", aggregate.HitRate[5]),
		"mrr", fmt.Sprintf("%.3f", aggregate.MRR),
		"avg_answer_score", fmt.Sprintf("%.3f", aggregate.AvgAnswerScore),
	)
	return nil
}
