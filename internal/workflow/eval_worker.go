package workflow

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type EvalArgs struct {
	Tag               string `json:"tag"`
	IndexTag          string `json:"index_tag"`
	QueryStrategy     string `json:"query_strategy"`
	DatasetPath       string `json:"dataset_path"`
	TopK              int    `json:"top_k"`
	Ks                []int  `json:"ks"`
	LLMProvider       string `json:"llm_provider"`
	LLMModel          string `json:"llm_model"`
	EmbeddingProvider string `json:"embedding_provider"`
	EmbeddingModel    string `json:"embedding_model"`
	JudgeProvider     string `json:"judge_provider"`
	JudgeModel        string `json:"judge_model"`
	Workers           int    `json:"workers"`
	BatchSize         int    `json:"batch_size"`
}

func (EvalArgs) Kind() string { return "eval" }

type workUnit struct {
	Question  types.EvalQuestion
	Embedding []float64
}

type evalCheckpoint struct {
	QuestionsProcessed int `json:"questions_processed"`
}

type EvalWorker struct {
	river.WorkerDefaults[EvalArgs]
	EvalStore *eval.EvalStore
	Client    *river.Client[pgx.Tx]
}

func NewEvalWorker(evalStore *eval.EvalStore) *EvalWorker {
	return &EvalWorker{EvalStore: evalStore}
}

func (w *EvalWorker) Work(ctx context.Context, job *river.Job[EvalArgs]) error {
	logger := slog.With("job_id", job.ID, "worker", "eval")
	logger.Debug("starting eval worker")

	args := job.Args
	cp := readEvalCheckpoint(job)

	workers := args.Workers
	if workers <= 0 {
		workers = 5
	}
	batchSize := args.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}

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

	// On retry, clear any previously stored results for this run
	if cp.QuestionsProcessed > 0 {
		if err := w.EvalStore.DeleteRunResults(ctx, evalRunID); err != nil {
			return fmt.Errorf("delete previous results: %w", err)
		}
	}

	// Create dependencies
	emb, qStore, gen, judgeGen, err := createEvalDeps(ctx, args)
	if err != nil {
		return err
	}
	defer qStore.Close()

	file, err := os.Open(args.DatasetPath)
	if err != nil {
		return fmt.Errorf("open dataset: %w", err)
	}
	defer file.Close()

	questionChan := make(chan types.EvalQuestion, batchSize)
	workChan := make(chan workUnit, batchSize)
	resultChan := make(chan types.RetrievalResult, batchSize)

	var allResults []types.RetrievalResult
	g, ctx := errgroup.WithContext(ctx)

	// Publisher: reads JSONL, sends to questionChan
	g.Go(func() error {
		defer close(questionChan)
		return publishQuestions(ctx, file, questionChan, cp.QuestionsProcessed)
	})

	// Embedder: batch embeds, sends to workChan
	g.Go(func() error {
		defer close(workChan)
		return embedQuestions(ctx, emb, questionChan, workChan, batchSize)
	})

	// Subscribers: evaluate questions
	for i := 0; i < workers; i++ {
		g.Go(func() error {
			return evaluateQuestions(ctx, args, qStore, gen, judgeGen, workChan, resultChan)
		})
	}

	// Collector: gathers results, stores to DB, aggregates at end
	g.Go(func() error {
		return collectResults(ctx, args, w.EvalStore, w.Client, job, evalRunID, resultChan, &allResults, batchSize)
	})

	return g.Wait()
}

func publishQuestions(ctx context.Context, file *os.File, questionChan chan<- types.EvalQuestion, skip int) error {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var seen int
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var q types.EvalQuestion
		if err := json.Unmarshal(line, &q); err != nil {
			slog.Warn("skipping malformed jsonl line", "err", err)
			continue
		}
		if q.ID == "" || q.Question == "" {
			continue
		}

		seen++
		if seen <= skip {
			continue
		}

		select {
		case questionChan <- q:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return scanner.Err()
}

func embedQuestions(ctx context.Context, emb embedder.Embedder, questionChan <-chan types.EvalQuestion, workChan chan<- workUnit, batchSize int) error {
	var lineNum int
	for {
		batch := make([]types.EvalQuestion, 0, batchSize)
		for i := 0; i < batchSize; i++ {
			select {
			case q, ok := <-questionChan:
				if !ok {
					if len(batch) == 0 {
						return nil
					}
					goto flush
				}
				batch = append(batch, q)
			case <-ctx.Done():
				return ctx.Err()
			}
		}

	flush:
		chunks := make([]types.Chunk, len(batch))
		for i, q := range batch {
			chunks[i] = types.Chunk{ID: q.ID, Content: q.Question}
		}

		embeddings, err := emb.Embed(ctx, chunks)
		if err != nil {
			return fmt.Errorf("embed batch: %w", err)
		}

		for i, q := range batch {
			lineNum++
			wu := workUnit{
				Question:  q,
				Embedding: embeddings[i].Vector,
			}
			select {
			case workChan <- wu:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if len(batch) < batchSize {
			return nil
		}
	}
}

func evaluateQuestions(ctx context.Context, args EvalArgs, qStore eval.VectorSearcher, gen generator.Generator, judgeGen generator.Generator, workChan <-chan workUnit, resultChan chan<- types.RetrievalResult) error {
	for {
		select {
		case wu, ok := <-workChan:
			if !ok {
				return nil
			}
			result, err := eval.EvaluateQuestion(ctx, wu.Question, wu.Embedding, qStore, gen, judgeGen, args.IndexTag, args.TopK)
			if err != nil {
				slog.Warn("evaluate question failed", "question_id", wu.Question.ID, "err", err)
				result = types.RetrievalResult{
					QuestionID: wu.Question.ID,
					Question:   wu.Question.Question,
					Failed:     true,
				}
			}
			select {
			case resultChan <- result:
			case <-ctx.Done():
				return ctx.Err()
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func collectResults(
	ctx context.Context,
	args EvalArgs,
	evalStore *eval.EvalStore,
	client *river.Client[pgx.Tx],
	job *river.Job[EvalArgs],
	evalRunID string,
	resultChan <-chan types.RetrievalResult,
	allResults *[]types.RetrievalResult,
	batchSize int,
) error {
	var results []types.RetrievalResult
	ks := args.Ks
	if len(ks) == 0 {
		ks = []int{1, 3, 5, 10}
	}

	for {
		select {
		case r, ok := <-resultChan:
			if !ok {
				// Final flush + aggregate
				if len(results) > 0 {
					if err := evalStore.BulkAddQueryResults(ctx, evalRunID, results); err != nil {
						return fmt.Errorf("store final batch: %w", err)
					}
					*allResults = append(*allResults, results...)
				}

				aggregate := eval.ComputeAggregateMetrics(*allResults, ks)
				var totalLatency int64
				var totalPrompt, totalCompletion int
				for _, r := range *allResults {
					totalLatency += r.LatencyMs
					totalPrompt += r.PromptTokens
					totalCompletion += r.CompletionTokens
				}
				aggregate.AvgLatencyMs = float64(totalLatency) / float64(max(len(*allResults), 1))
				aggregate.TotalLatencyMs = totalLatency
				aggregate.AvgPromptTokens = float64(totalPrompt) / float64(max(len(*allResults), 1))
				aggregate.AvgCompletionTokens = float64(totalCompletion) / float64(max(len(*allResults), 1))
				aggregate.TotalPromptTokens = totalPrompt
				aggregate.TotalCompletionTokens = totalCompletion

				if err := evalStore.UpdateRunMetrics(ctx, evalRunID, aggregate); err != nil {
					return fmt.Errorf("update metrics: %w", err)
				}

				slog.Info("evaluation complete",
					"questions", len(*allResults),
					"hit_rate@5", fmt.Sprintf("%.3f", aggregate.HitRate[5]),
					"mrr", fmt.Sprintf("%.3f", aggregate.MRR),
					"avg_answer_score", fmt.Sprintf("%.3f", aggregate.AvgAnswerScore),
				)
				return nil
			}

			results = append(results, r)
			if len(results) >= batchSize {
				if err := evalStore.BulkAddQueryResults(ctx, evalRunID, results); err != nil {
					return fmt.Errorf("store batch: %w", err)
				}
				*allResults = append(*allResults, results...)

				// Checkpoint
				processed := len(*allResults)
				if err := saveEvalCheckpoint(ctx, client, job, processed); err != nil {
					slog.Warn("failed to save checkpoint", "err", err)
				}
				results = results[:0]
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func createEvalDeps(ctx context.Context, args EvalArgs) (embedder.Embedder, *qstore.QdrantStore, generator.Generator, generator.Generator, error) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")

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
		return nil, nil, nil, nil, fmt.Errorf("create embedder: %w", err)
	}

	qStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := qStore.Connect(ctx, qdrantURL); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("connect qdrant: %w", err)
	}

	gen, err := generator.New(llmProvider, args.LLMModel)
	if err != nil {
		qStore.Close()
		return nil, nil, nil, nil, fmt.Errorf("create generator: %w", err)
	}

	judgeGen, err := generator.New(judgeProvider, args.JudgeModel)
	if err != nil {
		qStore.Close()
		return nil, nil, nil, nil, fmt.Errorf("create judge generator: %w", err)
	}

	return emb, qStore, gen, judgeGen, nil
}

func readEvalCheckpoint(job *river.Job[EvalArgs]) evalCheckpoint {
	raw := job.Output()
	if len(raw) == 0 {
		return evalCheckpoint{}
	}
	var cp evalCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		return evalCheckpoint{}
	}
	return cp
}

func saveEvalCheckpoint(ctx context.Context, client *river.Client[pgx.Tx], job *river.Job[EvalArgs], questionsProcessed int) error {
	_, err := client.JobUpdate(ctx, job.ID, &river.JobUpdateParams{
		Output: evalCheckpoint{QuestionsProcessed: questionsProcessed},
	})
	return err
}
