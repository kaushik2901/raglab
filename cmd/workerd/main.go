package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/workflow"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	if err := run(ctx); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	pool, err := db.Connect(ctx)
	if err != nil {
		return err
	}

	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return err
	}

	evalStore := eval.NewEvalStore(pool)

	preprocessWorker := &workflow.PreprocessWorker{}
	indexWorker := &workflow.IndexWorker{}
	evalWorker := workflow.NewEvalWorker(evalStore)

	workers := river.NewWorkers()
	river.AddWorker(workers, preprocessWorker)
	river.AddWorker(workers, indexWorker)
	river.AddWorker(workers, evalWorker)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		JobTimeout: 60 * time.Minute,
		Queues: map[string]river.QueueConfig{
			"default": {MaxWorkers: config.IntEnvOrDefault("WORKER_CONCURRENCY", 20)},
		},
		Workers: workers,
	})
	if err != nil {
		pool.Close()
		return err
	}

	preprocessWorker.Client = riverClient
	evalWorker.Client = riverClient

	if err := riverClient.Start(ctx); err != nil {
		pool.Close()
		return err
	}

	slog.Info("workerd started, waiting for jobs")

	<-ctx.Done()

	slog.Info("shutting down...")
	if err := riverClient.Stop(context.Background()); err != nil {
		slog.Error("stop failed", "err", err)
	}
	pool.Close()
	return nil
}
