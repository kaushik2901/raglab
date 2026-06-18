package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaushik2901/raglab/internal/chunker"
	"github.com/kaushik2901/raglab/internal/config"
	"github.com/kaushik2901/raglab/internal/embedder"
	"github.com/kaushik2901/raglab/internal/parser"
	qstore "github.com/kaushik2901/raglab/internal/store"
	"github.com/kaushik2901/raglab/internal/types"
	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"
)

type IndexArgs struct {
	Tag               string         `json:"tag"`
	InputTag          string         `json:"input_tag"`
	ParserStrategy    string         `json:"parser_strategy"`
	ChunkStrategy     string         `json:"chunk_strategy"`
	ChunkConfig       map[string]any `json:"chunk_config"`
	EmbeddingProvider string         `json:"embedding_provider"`
	EmbeddingModel    string         `json:"embedding_model"`
	BatchSize         int            `json:"batch_size"`
	IndexConcurrency  int            `json:"index_concurrency"`
	DocTimeout        time.Duration  `json:"doc_timeout"`
}

func (IndexArgs) Kind() string { return "index" }

type IndexWorker struct {
	river.WorkerDefaults[IndexArgs]
}

func (w *IndexWorker) Work(ctx context.Context, job *river.Job[IndexArgs]) error {
	logger := slog.With("job_id", job.ID, "worker", "index")
	logger.Debug("starting index worker")

	return RunIndexing(ctx, job.Args)
}

func embedAndStore(ctx context.Context, emb embedder.Embedder, qStore qstore.VectorStore, collectionName string, batch []types.Chunk) error {
	batchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	embeddings, err := emb.Embed(batchCtx, batch)
	if err != nil {
		return fmt.Errorf("embed batch: %w", err)
	}

	docChunks := make([]types.DocumentChunk, len(batch))
	for i := range batch {
		docChunks[i] = types.DocumentChunk{
			Chunk:     batch[i],
			Embedding: embeddings[i],
		}
	}

	if err := qStore.Store(batchCtx, collectionName, docChunks); err != nil {
		return fmt.Errorf("store batch: %w", err)
	}
	return nil
}

func processFile(ctx context.Context, fp, relPath, collectionName string,
	parsr parser.Parser, chunkr chunker.Chunker, emb embedder.Embedder, qStore qstore.VectorStore,
	batchSize int) (int, error) {

	reader, err := parsr.Parse(fp)
	if err != nil {
		return 0, fmt.Errorf("parse: %w", err)
	}
	defer reader.Close()

	chunkChan, errChan := chunkr.Chunk(ctx, reader, relPath)

	var (
		batch       []types.Chunk
		chunksCount int
	)
	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				if len(batch) > 0 {
					if err := embedAndStore(ctx, emb, qStore, collectionName, batch); err != nil {
						return chunksCount, err
					}
					chunksCount += len(batch)
				}
				return chunksCount, nil
			}
			batch = append(batch, chunk)
			if len(batch) >= batchSize {
				if err := embedAndStore(ctx, emb, qStore, collectionName, batch); err != nil {
					return chunksCount, err
				}
				chunksCount += len(batch)
				batch = batch[:0]
			}
		case err := <-errChan:
			if err == nil {
				continue
			}
			return chunksCount, fmt.Errorf("chunk: %w", err)
		}
	}
}

const maxIndexFileSize = 100 * 1024 * 1024

func RunIndexing(ctx context.Context, args IndexArgs) error {
	concurrency := args.IndexConcurrency

	inputDir := path.Join("artifacts", "preprocessing", args.InputTag, "output")

	embeddingProvider := config.Provider(args.EmbeddingProvider)

	emb, err := embedder.New(embeddingProvider, args.EmbeddingModel, args.BatchSize)
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}

	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
	parsr, err := parser.New(args.ParserStrategy)
	if err != nil {
		return fmt.Errorf("create parser: %w", err)
	}

	chunkr, err := chunker.New(args.ChunkStrategy, args.ChunkConfig)
	if err != nil {
		return fmt.Errorf("create chunker: %w", err)
	}

	rawStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := rawStore.Connect(ctx, qdrantURL); err != nil {
		return fmt.Errorf("connect qdrant: %w", err)
	}
	qStore := qstore.NewCircuitBreakerVectorStore(rawStore)
	defer rawStore.Close()

	collectionName := args.Tag

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
		return fmt.Errorf("walk input dir: %w", err)
	}

	slog.Info("indexing files", "count", len(mdFiles), "concurrency", concurrency)

	if len(mdFiles) > 0 {
		probeEmb, err := emb.Embed(ctx, []types.Chunk{{ID: "dimension-probe", Content: "test"}})
		if err != nil {
			return fmt.Errorf("probe embedding dimension: %w", err)
		}
		if len(probeEmb) == 0 {
			return fmt.Errorf("no embedding returned for dimension probe")
		}
		vectorSize := probeEmb[0].Dimensions

		if err := qStore.EnsureCollection(ctx, collectionName, vectorSize, "Cosine"); err != nil {
			return fmt.Errorf("ensure collection: %w", err)
		}
	}

	var (
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
			docCtx, cancel := context.WithTimeout(ctx, args.DocTimeout)
			defer cancel()

			relPath, err := filepath.Rel(inputDir, fp)
			if err != nil {
				return fmt.Errorf("relative path: %w", err)
			}
			relPath = filepath.ToSlash(relPath)

			fi, err := os.Stat(fp)
			if err != nil {
				mu.Lock()
				skipErrors = append(skipErrors, relPath+": stat: "+err.Error())
				mu.Unlock()
				return nil
			}
			maxSize := maxIndexFileSize
			if fi.Size() > int64(maxSize) {
				mu.Lock()
				skipErrors = append(skipErrors, relPath+": file too large ("+strconv.FormatInt(fi.Size(), 10)+" bytes, max "+strconv.Itoa(maxSize)+")")
				mu.Unlock()
				return nil
			}

			chunkCount, err := processFile(docCtx, fp, relPath, collectionName, parsr, chunkr, emb, qStore, args.BatchSize)
			if err != nil {
				mu.Lock()
				skipErrors = append(skipErrors, relPath+": "+err.Error())
				mu.Unlock()
				return nil
			}

			if chunkCount == 0 {
				mu.Lock()
				skipErrors = append(skipErrors, relPath+": empty chunks")
				mu.Unlock()
				return nil
			}

			totalDocs.Add(1)
			totalChunks.Add(int32(chunkCount))
			slog.Info("indexed document", "path", relPath, "chunks", chunkCount)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if len(skipErrors) > 0 {
		slog.Warn("indexing completed with skipped files",
			"indexed", totalDocs.Load(),
			"skipped", len(skipErrors),
			"errors", skipErrors)
	}

	return nil
}
