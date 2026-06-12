package workflow

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/retriever"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type EvalArgs struct {
	Tag               string `json:"tag"`
	IndexTag          string `json:"index_tag"`
	QueryStrategy     string `json:"query_strategy"`
	DatasetPath       string `json:"dataset_path"`
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
	RunID              string `json:"run_id"`
	QuestionsProcessed int    `json:"questions_processed"`
}

// EvalWorkerDeps holds all dependencies for the eval worker.
type EvalWorkerDeps struct {
	EvalDB    eval.EvalDB
	Client    *river.Client[pgx.Tx]
	Retriever *retriever.Retriever
	Embedder  embedder.Embedder
	Generator generator.Generator
	JudgeGen  generator.Generator
}

type EvalWorker struct {
	river.WorkerDefaults[EvalArgs]
	EvalDB    eval.EvalDB
	Client    *river.Client[pgx.Tx]
	Retriever *retriever.Retriever
	Embedder  embedder.Embedder
	Generator generator.Generator
	JudgeGen  generator.Generator
}

func NewEvalWorker(evalDB eval.EvalDB) *EvalWorker {
	return &EvalWorker{EvalDB: evalDB}
}

// NewEvalWorkerWithDeps creates a fully-initialized EvalWorker with all
// dependencies injected, enabling unit testing without real infrastructure.
func NewEvalWorkerWithDeps(deps EvalWorkerDeps) *EvalWorker {
	return &EvalWorker{
		EvalDB:    deps.EvalDB,
		Client:    deps.Client,
		Retriever: deps.Retriever,
		Embedder:  deps.Embedder,
		Generator: deps.Generator,
		JudgeGen:  deps.JudgeGen,
	}
}

func (w *EvalWorker) Work(ctx context.Context, job *river.Job[EvalArgs]) error {
	logger := slog.With("job_id", job.ID, "worker", "eval")
	logger.Debug("starting eval worker")

	args := job.Args
	cp := readEvalCheckpoint(job)

	workers := args.Workers
	batchSize := args.BatchSize
	topK := maxIntSlice(args.Ks)

	// On retry, clean up the previous run's results before creating a new one
	if cp.QuestionsProcessed > 0 && cp.RunID != "" {
		if err := w.EvalDB.DeleteRunResults(ctx, cp.RunID); err != nil {
			return fmt.Errorf("delete previous results: %w", err)
		}
	}

	evalRunID, err := w.EvalDB.CreateRun(ctx, args.Tag, map[string]any{
		"index_tag":      args.IndexTag,
		"query_strategy": args.QueryStrategy,
		"llm_provider":   args.LLMProvider,
		"llm_model":      args.LLMModel,
		"judge_provider": args.JudgeProvider,
		"judge_model":    args.JudgeModel,
	})
	if err != nil {
		return fmt.Errorf("create eval run: %w", err)
	}

	// Resolve dependencies: prefer injected, fall back to real creation
	emb := w.Embedder
	ret := w.Retriever
	gen := w.Generator
	judgeGen := w.JudgeGen
	var rawStore *qstore.QdrantStore
	if emb == nil || ret == nil || gen == nil {
		var cerr error
		emb, ret, rawStore, gen, judgeGen, cerr = createEvalDeps(ctx, args)
		if cerr != nil {
			return cerr
		}
		defer rawStore.Close()
	}

	datasetPath := resolveDatasetPath(args.DatasetPath)
	file, err := os.Open(datasetPath)
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
	var workerWg sync.WaitGroup
	for i := 0; i < workers; i++ {
		workerWg.Add(1)
		g.Go(func() error {
			defer workerWg.Done()
			return evaluateQuestions(ctx, args, topK, ret, gen, judgeGen, workChan, resultChan)
		})
	}

	// Closer: close resultChan when all workers complete
	g.Go(func() error {
		workerWg.Wait()
		close(resultChan)
		return nil
	})

	// Collector: gathers results, stores to DB, aggregates at end
	g.Go(func() error {
		return collectResults(ctx, args, w.EvalDB, w.Client, job, evalRunID, resultChan, &allResults, batchSize)
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
		channelOpen := true
		for i := 0; i < batchSize && channelOpen; i++ {
			select {
			case q, ok := <-questionChan:
				if !ok {
					channelOpen = false
					if len(batch) == 0 {
						return nil
					}
					break
				}
				batch = append(batch, q)
			case <-ctx.Done():
				return ctx.Err()
			}
		}

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

		if !channelOpen {
			return nil
		}
	}
}

func evaluateQuestions(ctx context.Context, args EvalArgs, topK int, ret *retriever.Retriever, gen generator.Generator, judgeGen generator.Generator, workChan <-chan workUnit, resultChan chan<- types.RetrievalResult) error {
	for {
		select {
		case wu, ok := <-workChan:
			if !ok {
				return nil
			}

			queryVector := eval.ToFloat32(wu.Embedding)
			searchResults, err := ret.Retrieve(ctx, args.IndexTag, queryVector, topK)
			if err != nil {
				slog.Warn("retrieve failed", "question_id", wu.Question.ID, "err", err)
				resultChan <- types.RetrievalResult{
					QuestionID: wu.Question.ID,
					Question:   wu.Question.Question,
					Failed:     true,
				}
				continue
			}

			result, err := eval.EvaluateQuestion(ctx, wu.Question, searchResults, gen, judgeGen, topK)
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
	evalDB eval.EvalDB,
	client *river.Client[pgx.Tx],
	job *river.Job[EvalArgs],
	evalRunID string,
	resultChan <-chan types.RetrievalResult,
	allResults *[]types.RetrievalResult,
	batchSize int,
) error {
	var results []types.RetrievalResult
	ks := args.Ks

	for {
		select {
		case r, ok := <-resultChan:
			if !ok {
				// Final flush + aggregate
				if len(results) > 0 {
					if err := evalDB.BulkAddQueryResults(ctx, evalRunID, results); err != nil {
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

				if err := evalDB.UpdateRunMetrics(ctx, evalRunID, aggregate); err != nil {
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
				if err := evalDB.BulkAddQueryResults(ctx, evalRunID, results); err != nil {
					return fmt.Errorf("store batch: %w", err)
				}
				*allResults = append(*allResults, results...)

				// Checkpoint
				processed := len(*allResults)
				if err := saveEvalCheckpoint(ctx, client, job, evalRunID, processed); err != nil {
					slog.Warn("failed to save checkpoint", "err", err)
				}
				results = results[:0]
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func createEvalDeps(ctx context.Context, args EvalArgs) (embedder.Embedder, *retriever.Retriever, *qstore.QdrantStore, generator.Generator, generator.Generator, error) {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")

	llmProvider := config.Provider(args.LLMProvider)
	embeddingProvider := config.Provider(args.EmbeddingProvider)
	judgeProvider := config.Provider(args.JudgeProvider)

	embeddingModel := args.EmbeddingModel

	emb, err := embedder.New(embeddingProvider, embeddingModel, args.BatchSize)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("create embedder: %w", err)
	}

	rawStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := rawStore.Connect(ctx, qdrantURL); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("connect qdrant: %w", err)
	}

	cbStore := qstore.NewCircuitBreakerVectorStore(rawStore)

	ret, err := retriever.New(cbStore, args.QueryStrategy)
	if err != nil {
		rawStore.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("create retriever: %w", err)
	}

	gen, err := generator.New(llmProvider, args.LLMModel)
	if err != nil {
		rawStore.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("create generator: %w", err)
	}

	judgeGen, err := generator.New(judgeProvider, args.JudgeModel)
	if err != nil {
		rawStore.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("create judge generator: %w", err)
	}

	return emb, ret, rawStore, gen, judgeGen, nil
}

func resolveDatasetPath(datasetPath string) string {
	if filepath.IsAbs(datasetPath) {
		return datasetPath
	}
	datasetsDir := os.Getenv("DATASETS_DIR")
	if datasetsDir == "" {
		datasetsDir = "/workspace/artifacts/datasets"
	}
	return filepath.Join(datasetsDir, datasetPath)
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

func saveEvalCheckpoint(ctx context.Context, client *river.Client[pgx.Tx], job *river.Job[EvalArgs], runID string, questionsProcessed int) error {
	_, err := client.JobUpdate(ctx, job.ID, &river.JobUpdateParams{
		Output: evalCheckpoint{RunID: runID, QuestionsProcessed: questionsProcessed},
	})
	return err
}

func maxIntSlice(vals []int) int {
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
