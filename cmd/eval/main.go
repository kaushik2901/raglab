package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/riverqueue/river/rivertype"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
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
	queryStrategy := flag.String("query-strategy", "", "Query strategy (required)")
	dataset := flag.String("dataset", "", "Path to .jsonl dataset file (required)")
	topK := flag.Int("top-k", 5, "Top-K retrieval")
	ksRaw := flag.String("ks", "1,3,5,10", "Comma-separated list of K values for metrics")
	llmProvider := flag.String("llm-provider", config.EnvOrDefault("LLM_PROVIDER", "openai"), "LLM provider (openai, gemini, openrouter, lmstudio)")
	llmModel := flag.String("llm-model", config.EnvOrDefault("LLM_MODEL", "gpt-4o-mini"), "LLM model for answer generation")
	embeddingProvider := flag.String("embedding-provider", config.EnvOrDefault("EMBEDDING_PROVIDER", ""), "Embedding provider (defaults to --llm-provider if empty)")
	embeddingModel := flag.String("embedding-model", config.EnvOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"), "Embedding model for query vectorization")
	judgeProvider := flag.String("judge-provider", config.EnvOrDefault("JUDGE_PROVIDER", ""), "Judge provider (defaults to --llm-provider if empty)")
	judgeModel := flag.String("judge-model", config.EnvOrDefault("JUDGE_MODEL", ""), "LLM model for answer scoring (defaults to --llm-model if empty)")
	workers := flag.Int("workers", config.IntEnvOrDefault("WORKERS", 5), "Number of concurrent evaluator goroutines")
	batchSize := flag.Int("batch-size", config.IntEnvOrDefault("BATCH_SIZE", 20), "Embedding API batch size")
	tag := flag.String("tag", config.EnvOrDefault("TAG", ""), "Eval run tag (auto-generated if empty)")

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
	if *dataset == "" {
		return fmt.Errorf("--dataset is required")
	}

	if _, err := os.Stat(*dataset); err != nil {
		return fmt.Errorf("dataset file not found: %s", *dataset)
	}

	var ks []int
	for _, s := range strings.Split(*ksRaw, ",") {
		k, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("invalid value in --ks: %q", s)
		}
		ks = append(ks, k)
	}

	if *embeddingProvider == "" {
		*embeddingProvider = *llmProvider
	}
	if *judgeProvider == "" {
		*judgeProvider = *llmProvider
	}
	if *judgeModel == "" {
		*judgeModel = *llmModel
	}

	ctx := context.Background()

	rc, err := db.NewRiverClient(ctx, cfg.MaxRetries+1)
	if err != nil {
		return err
	}
	defer rc.Pool.Close()

	resolvedTag := config.ResolveTag(*tag, "eval")

	result, err := rc.Client.Insert(ctx, &workflow.EvalArgs{
		Tag:               resolvedTag,
		IndexTag:          *indexTag,
		QueryStrategy:     *queryStrategy,
		DatasetPath:       *dataset,
		TopK:              *topK,
		Ks:                ks,
		LLMProvider:       *llmProvider,
		LLMModel:          *llmModel,
		EmbeddingProvider: *embeddingProvider,
		EmbeddingModel:    *embeddingModel,
		JudgeProvider:     *judgeProvider,
		JudgeModel:        *judgeModel,
		Workers:           *workers,
		BatchSize:         *batchSize,
	}, nil)
	if err != nil {
		return fmt.Errorf("insert eval job: %w", err)
	}

	jobID := result.Job.ID
	slog.Info("submitted eval job", "id", jobID, "tag", resolvedTag)

	row, err := workflow.PollUntilTerminal(ctx, rc.Client, jobID, 2*time.Second)
	if err != nil {
		return fmt.Errorf("eval job %d: %w", jobID, err)
	}
	if row.State != rivertype.JobStateCompleted {
		return fmt.Errorf("eval job %d failed: state=%s errors=%v", jobID, row.State, row.Errors)
	}

	slog.Info("evaluation complete", "tag", resolvedTag)
	return nil
}
