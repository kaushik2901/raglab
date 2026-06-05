package main

import (
	"context"
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
	llmModel := flag.String("llm-model", config.EnvOrDefault("LLM_MODEL", "gpt-4o-mini"), "LLM model for answer generation")
	judgeModel := flag.String("judge-model", config.EnvOrDefault("JUDGE_MODEL", ""), "LLM model for answer scoring (defaults to --llm-model if empty)")
	evalConcurrency := flag.Int("eval-concurrency", config.IntEnvOrDefault("EVAL_CONCURRENCY", 5), "Number of questions to evaluate concurrently")
	tag := flag.String("tag", "", "Eval run tag prefix (auto-generated if empty)")

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

	info, err := os.Stat(*datasetDir)
	if err != nil {
		return fmt.Errorf("dataset directory not found: %s", *datasetDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("--dataset-dir must be a directory: %s", *datasetDir)
	}

	entries, err := os.ReadDir(*datasetDir)
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

		wfID, err := store.CreateWorkflow(ctx, "eval", fileTag, map[string]any{
			"index_tag":      *indexTag,
			"query_strategy": *queryStrategy,
			"dataset_path":   filepath.Join(*datasetDir, f),
			"top_k":          *topK,
			"llm_model":      *llmModel,
			"judge_model":    *judgeModel,
			"concurrency":    *evalConcurrency,
		})
		if err != nil {
			return fmt.Errorf("create workflow for %s: %w", f, err)
		}

		_, err = riverClient.Insert(ctx, &workflow.EvalArgs{
			WorkflowID:    wfID,
			Tag:           fileTag,
			IndexTag:      *indexTag,
			QueryStrategy: *queryStrategy,
			DatasetPath:   filepath.Join(*datasetDir, f),
			TopK:          *topK,
			LLMModel:      *llmModel,
			JudgeModel:    *judgeModel,
			Concurrency:   *evalConcurrency,
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
