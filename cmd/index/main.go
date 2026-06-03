package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/journal"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/pipeline"
	stagepkg "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/stage"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fromStage := extractFromFlag(args)
	cleanArgs := stripFromFlag(args)

	origArgs := os.Args
	os.Args = append([]string{"index"}, cleanArgs...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	cfg, err := config.Load()
	os.Args = origArgs
	if err != nil {
		return err
	}

	p := buildPipeline(cfg)

	if fromStage != "" {
		return p.RunFrom(context.Background(), types.StageID(fromStage))
	}
	return p.Run(context.Background())
}

func extractFromFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--from" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func stripFromFlag(args []string) []string {
	var result []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--from" {
			i++
			continue
		}
		result = append(result, args[i])
	}
	return result
}

func buildPipeline(cfg *config.Config) pipeline.Pipeline {
	return pipeline.Pipeline{
		Journal: journal.NewGobFileJournal(".journal-index"),
		Config:  cfg,
		Stages: []pipeline.Stage{
			stagepkg.ParseStage(cfg),
			stagepkg.ChunkStage(cfg),
			stagepkg.EmbedStage(cfg),
			stagepkg.StoreStage(cfg),
		},
	}
}
