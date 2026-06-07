package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/chunker"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/parser"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"
)

type IndexArgs struct {
	WorkflowID        string `json:"workflow_id"`
	Tag               string `json:"tag"`
	InputTag          string `json:"input_tag"`
	ChunkStrategy     string `json:"chunk_strategy"`
	ChunkSize         int    `json:"chunk_size"`
	ChunkOverlap      int    `json:"chunk_overlap"`
	EmbeddingProvider string `json:"embedding_provider"`
	EmbeddingModel    string `json:"embedding_model"`
	BatchSize         int    `json:"batch_size"`
	IndexConcurrency  int    `json:"index_concurrency"`
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
	logger := slog.With("workflow_id", job.Args.WorkflowID, "worker", "index")
	logger.Debug("starting index worker")

	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "index", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		return RunIndexing(ctx, job.Args)
	}); err != nil {
		return err
	}

	return w.Store.UpdateWorkflowStatus(ctx, job.Args.WorkflowID, "succeeded")
}

func RunIndexing(ctx context.Context, args IndexArgs) (*types.StageResult, error) {
	concurrency := args.IndexConcurrency
	if concurrency <= 0 {
		concurrency = 5
	}

	inputDir := path.Join("artifacts", "preprocessing", args.InputTag, "output")

	embeddingProvider := config.Provider(args.EmbeddingProvider)
	if embeddingProvider == "" {
		embeddingProvider = config.ProviderOpenAI
	}

	emb, err := embedder.New(embeddingProvider, args.EmbeddingModel, args.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("create embedder: %w", err)
	}

	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
	chunkr := chunker.NewFixedChunker(args.ChunkSize, args.ChunkOverlap)

	qStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := qStore.Connect(ctx, qdrantURL); err != nil {
		return nil, fmt.Errorf("connect qdrant: %w", err)
	}
	defer qStore.Close()

	collectionName := args.Tag

	// Phase 1: collect all .md file paths
	var mdFiles []string
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
		mdFiles = append(mdFiles, path)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk input dir: %w", err)
	}

	slog.Info("indexing files", "count", len(mdFiles), "concurrency", concurrency)

	var (
		ensureOnce  sync.Once
		initErr     error
		totalDocs   atomic.Int32
		totalChunks atomic.Int32
		mu          sync.Mutex
		skipErrors  []string
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, filePath := range mdFiles {
		fp := filePath
		g.Go(func() error {
			docCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			relPath, err := filepath.Rel(inputDir, fp)
			if err != nil {
				return fmt.Errorf("relative path: %w", err)
			}
			relPath = filepath.ToSlash(relPath)

			doc, err := parser.ParseFile(fp, relPath)
			if err != nil {
				slog.Warn("parse error", "path", fp, "err", err)
				mu.Lock()
				skipErrors = append(skipErrors, relPath+": parse: "+err.Error())
				mu.Unlock()
				return nil
			}

			chunks, err := chunkr.Chunk(doc)
			if err != nil {
				slog.Warn("chunk error", "path", fp, "err", err)
				mu.Lock()
				skipErrors = append(skipErrors, relPath+": chunk: "+err.Error())
				mu.Unlock()
				return nil
			}
			if len(chunks) == 0 {
				mu.Lock()
				skipErrors = append(skipErrors, relPath+": empty chunks")
				mu.Unlock()
				return nil
			}

			embeddings, err := emb.Embed(docCtx, chunks)
			if err != nil {
				return fmt.Errorf("embed %s: %w", relPath, err)
			}

			ensureOnce.Do(func() {
				vectorSize := embeddings[0].Dimensions
				initErr = qStore.EnsureCollection(docCtx, collectionName, vectorSize, "Cosine")
			})
			if initErr != nil {
				return fmt.Errorf("ensure collection: %w", initErr)
			}

			docChunks := make([]types.DocumentChunk, len(chunks))
			for i := range chunks {
				docChunks[i] = types.DocumentChunk{
					Chunk:     chunks[i],
					Embedding: embeddings[i],
				}
			}

			if err := qStore.Store(docCtx, collectionName, docChunks); err != nil {
				return fmt.Errorf("store %s: %w", relPath, err)
			}

			totalDocs.Add(1)
			totalChunks.Add(int32(len(chunks)))
			slog.Info("indexed document", "path", relPath, "chunks", len(chunks))
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(skipErrors) > 0 {
		slog.Warn("indexing completed with skipped files",
			"indexed", totalDocs.Load(),
			"skipped", len(skipErrors),
			"errors", skipErrors)
	}

	return &types.StageResult{
		Name: "index",
		Output: map[string]any{
			"document_count": totalDocs.Load(),
			"chunk_count":    totalChunks.Load(),
			"collection":     collectionName,
		},
	}, nil
}
