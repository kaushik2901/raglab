package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"golang.org/x/sync/errgroup"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/retriever"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/workflow"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	flag.CommandLine = flag.NewFlagSet("eval", flag.ExitOnError)

	indexTag := flag.String("index-tag", "", "Existing Qdrant collection name to evaluate (required)")
	queryStrategy := flag.String("query-strategy", "", fmt.Sprintf("Query strategy (supported: %s)", retriever.StrategyNaiveSearch))
	datasetDir := flag.String("dataset-dir", "", "Path to evaluation dataset directory containing .json files (required)")
	topK := flag.Int("top-k", 5, "Top-K retrieval")
	llmProvider := flag.String("llm-provider", config.EnvOrDefault("LLM_PROVIDER", "openai"), "LLM provider (openai, gemini, openrouter, lmstudio)")
	llmModel := flag.String("llm-model", config.EnvOrDefault("LLM_MODEL", "gpt-4o-mini"), "LLM model for answer generation")
	embeddingProvider := flag.String("embedding-provider", config.EnvOrDefault("EMBEDDING_PROVIDER", ""), "Embedding provider (defaults to --llm-provider if empty)")
	embeddingModel := flag.String("embedding-model", config.EnvOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"), "Embedding model for query vectorization")
	judgeProvider := flag.String("judge-provider", config.EnvOrDefault("JUDGE_PROVIDER", ""), "Judge provider (defaults to --llm-provider if empty)")
	judgeModel := flag.String("judge-model", config.EnvOrDefault("JUDGE_MODEL", ""), "LLM model for answer scoring (defaults to --llm-model if empty)")
	evalConcurrency := flag.Int("eval-concurrency", config.IntEnvOrDefault("EVAL_CONCURRENCY", 5), "Number of questions to evaluate concurrently (currently sequential — reserved for future use)")
	batchSize := flag.Int("batch-size", config.IntEnvOrDefault("BATCH_SIZE", 20), "Embedding API batch size")
	tag := flag.String("tag", config.EnvOrDefault("TAG", ""), "Eval run tag prefix (auto-generated if empty)")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if *indexTag == "" {
		return fmt.Errorf("--index-tag is required")
	}
	if *queryStrategy == "" {
		return fmt.Errorf("--query-strategy is required")
	}
	if *datasetDir == "" {
		return fmt.Errorf("--dataset-dir is required")
	}

	absDir, err := filepath.Abs(*datasetDir)
	if err != nil {
		return fmt.Errorf("resolve dataset dir: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return fmt.Errorf("dataset directory not found: %s", absDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("--dataset-dir must be a directory: %s", absDir)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	relDir, err := filepath.Rel(cwd, absDir)
	if err != nil {
		return fmt.Errorf("relative dataset dir: %w", err)
	}
	// Forward-slash path for cross-platform compatibility (works locally + in Docker)
	*datasetDir = filepath.ToSlash(relDir)

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return fmt.Errorf("read dataset directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("no .json files found in %s", *datasetDir)
	}

	for _, f := range files {
		if err := validateDatasetFile(filepath.Join(absDir, f)); err != nil {
			return fmt.Errorf("invalid dataset %s: %w", f, err)
		}
	}

	ctx := context.Background()

	pool, err := db.Connect(ctx)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("db migrate: %w", err)
	}

	store := workflow.NewStore(pool)

	if *embeddingProvider == "" {
		*embeddingProvider = *llmProvider
	}
	if *judgeProvider == "" {
		*judgeProvider = *llmProvider
	}
	if *judgeModel == "" {
		*judgeModel = *llmModel
	}

	resolvedTagPrefix := config.ResolveTag(*tag, "eval")

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		MaxAttempts: cfg.MaxRetries + 1,
	})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	type fileWorkflow struct {
		file string
		tag  string
		wfID string
	}

	var workflows []fileWorkflow
	for _, f := range files {
		fileTag := resolvedTagPrefix + "-" + strings.TrimSuffix(f, ".json")

		datasetPath := filepath.ToSlash(filepath.Join(*datasetDir, f))

		wfID, err := store.CreateWorkflow(ctx, "eval", fileTag, map[string]any{
			"index_tag":          *indexTag,
			"main_tag":           resolvedTagPrefix,
			"query_strategy":     *queryStrategy,
			"dataset_path":       datasetPath,
			"top_k":              *topK,
			"llm_provider":       *llmProvider,
			"llm_model":          *llmModel,
			"embedding_provider": *embeddingProvider,
			"embedding_model":    *embeddingModel,
			"judge_provider":     *judgeProvider,
			"judge_model":        *judgeModel,
			"concurrency":        *evalConcurrency,
			"batch_size":         *batchSize,
		})
		if err != nil {
			return fmt.Errorf("create workflow for %s: %w", f, err)
		}

		_, err = riverClient.Insert(ctx, &workflow.EvalArgs{
			WorkflowID:        wfID,
			Tag:               fileTag,
			MainTag:           resolvedTagPrefix,
			IndexTag:          *indexTag,
			QueryStrategy:     *queryStrategy,
			DatasetPath:       datasetPath,
			TopK:              *topK,
			LLMProvider:       *llmProvider,
			LLMModel:          *llmModel,
			EmbeddingProvider: *embeddingProvider,
			EmbeddingModel:    *embeddingModel,
			JudgeProvider:     *judgeProvider,
			JudgeModel:        *judgeModel,
			Concurrency:       *evalConcurrency,
			BatchSize:         *batchSize,
		}, nil)
		if err != nil {
			return fmt.Errorf("insert eval job for %s: %w", f, err)
		}

		slog.Info("submitted eval workflow", "file", f, "id", wfID, "tag", fileTag)
		workflows = append(workflows, fileWorkflow{file: f, tag: fileTag, wfID: wfID})
	}

	g, ctx := errgroup.WithContext(ctx)
	for _, wf := range workflows {
		wf := wf
		g.Go(func() error {
			slog.Info("waiting for eval workflow", "file", wf.file, "id", wf.wfID, "tag", wf.tag)
			if err := workflow.PollUntilDone(ctx, store, wf.wfID, 2*time.Second); err != nil {
				return fmt.Errorf("file %s: %w", wf.file, err)
			}
			slog.Info("evaluation complete", "file", wf.file, "tag", wf.tag)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	slog.Info("all evaluations complete", "count", len(workflows))
	return nil
}

func validateDatasetFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	// Strip UTF-8 BOM if present
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var ds types.EvalDataset
	if err := json.Unmarshal(data, &ds); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}

	if len(ds.Questions) == 0 {
		return fmt.Errorf("dataset has no questions")
	}

	for i, q := range ds.Questions {
		if q.ID == "" {
			return fmt.Errorf("question %d: missing id", i)
		}
		if q.Question == "" {
			return fmt.Errorf("question %d (%s): missing question text", i, q.ID)
		}
		for j, rel := range q.Relevance {
			if rel.DocumentPath == "" {
				return fmt.Errorf("question %d (%s): relevance[%d] missing document_path", i, q.ID, j)
			}
		}
	}

	return nil
}
