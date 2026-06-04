package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

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
	dataset := flag.String("dataset", "testdata/eval/questions.json", "Path to ground truth questions JSON")
	topK := flag.Int("top-k", 5, "Top-K retrieval")
	llmModel := flag.String("llm-model", config.EnvOrDefault("LLM_MODEL", "gpt-4o-mini"), "LLM model for answer generation")
	tag := flag.String("tag", "", "Eval run tag (auto-generated if empty)")

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

	resolvedTag := config.ResolveTag(*tag, "eval")
	wfID, err := store.CreateWorkflow(ctx, "eval", resolvedTag, map[string]any{
		"index_tag":       *indexTag,
		"query_strategy":  *queryStrategy,
		"dataset_path":    *dataset,
		"top_k":           *topK,
		"llm_model":       *llmModel,
	})
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	slog.Info("created eval workflow", "id", wfID, "tag", resolvedTag)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		MaxAttempts: cfg.MaxRetries + 1,
	})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	_, err = riverClient.Insert(ctx, &workflow.EvalArgs{
		WorkflowID:    wfID,
		Tag:           resolvedTag,
		IndexTag:      *indexTag,
		QueryStrategy: *queryStrategy,
		DatasetPath:   *dataset,
		TopK:          *topK,
		LLMModel:      *llmModel,
	}, nil)
	if err != nil {
		return fmt.Errorf("insert eval job: %w", err)
	}

	slog.Info("inserted eval job, waiting for completion")
	if err := workflow.PollUntilDone(ctx, store, wfID, 2*time.Second); err != nil {
		return err
	}
	slog.Info("evaluation complete", "tag", resolvedTag)
	return nil
}
