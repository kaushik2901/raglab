package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/db"
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

	store := workflow.NewStore(pool)

	cloneWorker := &workflow.CloneWorker{Store: store}
	preprocessWorker := &workflow.PreprocessWorker{Store: store}
	verifyWorker := &workflow.VerifyWorker{Store: store}
	parseWorker := &workflow.ParseWorker{Store: store}
	chunkWorker := &workflow.ChunkWorker{Store: store}
	embedWorker := &workflow.EmbedWorker{Store: store}
	storeWorker := &workflow.StoreWorker{Store: store}

	workers := river.NewWorkers()
	river.AddWorker(workers, cloneWorker)
	river.AddWorker(workers, preprocessWorker)
	river.AddWorker(workers, verifyWorker)
	river.AddWorker(workers, parseWorker)
	river.AddWorker(workers, chunkWorker)
	river.AddWorker(workers, embedWorker)
	river.AddWorker(workers, storeWorker)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		JobTimeout: 5 * time.Minute,
		Queues: map[string]river.QueueConfig{
			"default": {MaxWorkers: 5},
		},
		Workers: workers,
	})
	if err != nil {
		pool.Close()
		return err
	}

	cloneWorker.Client = riverClient
	preprocessWorker.Client = riverClient
	verifyWorker.Client = riverClient
	parseWorker.Client = riverClient
	chunkWorker.Client = riverClient
	embedWorker.Client = riverClient
	storeWorker.Client = riverClient

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
