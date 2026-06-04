package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/chunker"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/parser"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
	"github.com/riverqueue/river"
)

type IndexArgs struct {
	WorkflowID     string `json:"workflow_id"`
	Tag            string `json:"tag"`
	InputTag       string `json:"input_tag"`
	ChunkStrategy  string `json:"chunk_strategy"`
	ChunkSize      int    `json:"chunk_size"`
	ChunkOverlap   int    `json:"chunk_overlap"`
	EmbeddingModel string `json:"embedding_model"`
	BatchSize      int    `json:"batch_size"`
}

func (IndexArgs) Kind() string { return "index" }

type IndexWorker struct {
	river.WorkerDefaults[IndexArgs]
	Store  *Store
	Client *river.Client[pgx.Tx]
}

func NewIndexWorker(store *Store, client *river.Client[pgx.Tx]) *IndexWorker {
	return &IndexWorker{Store: store, Client: client}
}

func (w *IndexWorker) Work(ctx context.Context, job *river.Job[IndexArgs]) error {
	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "index", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		return RunIndexing(ctx, job.Args)
	}); err != nil {
		return err
	}

	return w.Store.UpdateWorkflowStatus(ctx, job.Args.WorkflowID, "succeeded")
}

func RunIndexing(ctx context.Context, args IndexArgs) (*types.StageResult, error) {
	inputDir := path.Join("artifacts", "preprocessing", args.InputTag, "output")

	llmBaseURL := os.Getenv("LLM_BASE_URL")
	if llmBaseURL == "" {
		llmBaseURL = "https://api.openai.com/v1"
	}
	llmAPIKey := os.Getenv("LLM_API_KEY")
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")

	embedder := embedder.New(llmBaseURL, llmAPIKey, args.EmbeddingModel, args.BatchSize)
	chunker := chunker.NewFixedChunker(args.ChunkSize, args.ChunkOverlap)

	qStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := qStore.Connect(ctx, qdrantURL); err != nil {
		return nil, fmt.Errorf("connect qdrant: %w", err)
	}
	defer qStore.Close()

	collectionName := args.Tag
	var vectorSize int

	totalDocs := 0
	totalChunks := 0

	if err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk error", "path", path, "err", err)
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		relPath = filepath.ToSlash(relPath)

		doc, err := parser.ParseFile(path, relPath)
		if err != nil {
			slog.Warn("parse error", "path", path, "err", err)
			return nil
		}

		chunks, err := chunker.Chunk(doc)
		if err != nil {
			slog.Warn("chunk error", "path", path, "err", err)
			return nil
		}
		if len(chunks) == 0 {
			return nil
		}

		embeddings, err := embedder.Embed(ctx, chunks)
		if err != nil {
			return fmt.Errorf("embed %s: %w", relPath, err)
		}

		if vectorSize == 0 {
			vectorSize = embeddings[0].Dimensions
			if err := qStore.EnsureCollection(ctx, collectionName, vectorSize, "Cosine"); err != nil {
				return fmt.Errorf("ensure collection: %w", err)
			}
		}

		docChunks := make([]types.DocumentChunk, len(chunks))
		for i := range chunks {
			docChunks[i] = types.DocumentChunk{
				Chunk:     chunks[i],
				Embedding: embeddings[i],
			}
		}

		if err := qStore.Store(ctx, collectionName, docChunks); err != nil {
			return fmt.Errorf("store %s: %w", relPath, err)
		}

		totalDocs++
		totalChunks += len(chunks)
		slog.Info("indexed document", "path", relPath, "chunks", len(chunks))
		return nil
	}); err != nil {
		return nil, err
	}

	return &types.StageResult{
		Name: "index",
		Output: map[string]any{
			"document_count": totalDocs,
			"chunk_count":    totalChunks,
			"collection":     collectionName,
		},
	}, nil
}
