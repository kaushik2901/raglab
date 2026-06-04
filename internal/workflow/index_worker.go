package workflow

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/jackc/pgx/v5"
	stagepkg "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/stage"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
	"github.com/riverqueue/river"
)

type ParseArgs struct {
	WorkflowID     string `json:"workflow_id"`
	Tag            string `json:"tag"`
	InputTag       string `json:"input_tag"`
	ChunkStrategy  string `json:"chunk_strategy"`
	ChunkSize      int    `json:"chunk_size"`
	ChunkOverlap   int    `json:"chunk_overlap"`
	EmbeddingModel string `json:"embedding_model"`
	BatchSize      int    `json:"batch_size"`
}

func (ParseArgs) Kind() string { return "parse" }

type ChunkArgs struct {
	WorkflowID     string `json:"workflow_id"`
	Tag            string `json:"tag"`
	InputTag       string `json:"input_tag"`
	ChunkStrategy  string `json:"chunk_strategy"`
	ChunkSize      int    `json:"chunk_size"`
	ChunkOverlap   int    `json:"chunk_overlap"`
	EmbeddingModel string `json:"embedding_model"`
	BatchSize      int    `json:"batch_size"`
}

func (ChunkArgs) Kind() string { return "chunk" }

type EmbedArgs struct {
	WorkflowID     string `json:"workflow_id"`
	Tag            string `json:"tag"`
	InputTag       string `json:"input_tag"`
	EmbeddingModel string `json:"embedding_model"`
	BatchSize      int    `json:"batch_size"`
	LLMBaseURL     string `json:"llm_base_url"`
	LLMApiKey      string `json:"llm_api_key"`
}

func (EmbedArgs) Kind() string { return "embed" }

type StoreArgs struct {
	WorkflowID   string `json:"workflow_id"`
	Tag          string `json:"tag"`
	InputTag     string `json:"input_tag"`
	QdrantURL    string `json:"qdrant_url"`
	QdrantAPIKey string `json:"qdrant_api_key"`
}

func (StoreArgs) Kind() string { return "store" }

type ParseWorker struct {
	river.WorkerDefaults[ParseArgs]
	Store  *Store
	Client *river.Client[pgx.Tx]
}

func NewParseWorker(store *Store, client *river.Client[pgx.Tx]) *ParseWorker {
	return &ParseWorker{Store: store, Client: client}
}

func (w *ParseWorker) Work(ctx context.Context, job *river.Job[ParseArgs]) error {
	inputPath := path.Join("artifacts", "preprocessing", job.Args.InputTag, "output")

	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "parse", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		return stagepkg.ParseStage(inputPath).Run(ctx, state)
	}); err != nil {
		return err
	}

	_, err := w.Client.Insert(ctx, &ChunkArgs{
		WorkflowID:     job.Args.WorkflowID,
		Tag:            job.Args.Tag,
		InputTag:       job.Args.InputTag,
		ChunkStrategy:  job.Args.ChunkStrategy,
		ChunkSize:      job.Args.ChunkSize,
		ChunkOverlap:   job.Args.ChunkOverlap,
		EmbeddingModel: job.Args.EmbeddingModel,
		BatchSize:      job.Args.BatchSize,
	}, nil)
	return err
}

type ChunkWorker struct {
	river.WorkerDefaults[ChunkArgs]
	Store  *Store
	Client *river.Client[pgx.Tx]
}

func NewChunkWorker(store *Store, client *river.Client[pgx.Tx]) *ChunkWorker {
	return &ChunkWorker{Store: store, Client: client}
}

func (w *ChunkWorker) Work(ctx context.Context, job *river.Job[ChunkArgs]) error {
	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "chunk", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		return stagepkg.ChunkStage(job.Args.ChunkStrategy, job.Args.ChunkSize, job.Args.ChunkOverlap).Run(ctx, state)
	}); err != nil {
		return err
	}

	llmBaseURL := os.Getenv("LLM_BASE_URL")
	if llmBaseURL == "" {
		llmBaseURL = "https://api.openai.com/v1"
	}
	_, err := w.Client.Insert(ctx, &EmbedArgs{
		WorkflowID:     job.Args.WorkflowID,
		Tag:            job.Args.Tag,
		InputTag:       job.Args.InputTag,
		EmbeddingModel: job.Args.EmbeddingModel,
		BatchSize:      job.Args.BatchSize,
		LLMBaseURL:     llmBaseURL,
		LLMApiKey:      os.Getenv("LLM_API_KEY"),
	}, nil)
	return err
}

type EmbedWorker struct {
	river.WorkerDefaults[EmbedArgs]
	Store  *Store
	Client *river.Client[pgx.Tx]
}

func NewEmbedWorker(store *Store, client *river.Client[pgx.Tx]) *EmbedWorker {
	return &EmbedWorker{Store: store, Client: client}
}

func (w *EmbedWorker) Work(ctx context.Context, job *river.Job[EmbedArgs]) error {
	apiKey := job.Args.LLMApiKey
	if apiKey == "" {
		apiKey = os.Getenv("LLM_API_KEY")
	}

	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "embed", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		return stagepkg.EmbedStage(job.Args.LLMBaseURL, apiKey, job.Args.EmbeddingModel, job.Args.BatchSize).Run(ctx, state)
	}); err != nil {
		return err
	}

	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
	_, err := w.Client.Insert(ctx, &StoreArgs{
		WorkflowID:   job.Args.WorkflowID,
		Tag:          job.Args.Tag,
		InputTag:     job.Args.InputTag,
		QdrantURL:    qdrantURL,
		QdrantAPIKey: qdrantAPIKey,
	}, nil)
	return err
}

type StoreWorker struct {
	river.WorkerDefaults[StoreArgs]
	Store  *Store
	Client *river.Client[pgx.Tx]
}

func NewStoreWorker(store *Store, client *river.Client[pgx.Tx]) *StoreWorker {
	return &StoreWorker{Store: store, Client: client}
}

func (w *StoreWorker) Work(ctx context.Context, job *river.Job[StoreArgs]) error {
	if err := w.Store.runStep(ctx, job.Args.WorkflowID, "store", func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
		docChunks, ok := state["document_chunks"].([]types.DocumentChunk)
		if !ok {
			return nil, fmt.Errorf("document_chunks not found in state")
		}
		if len(docChunks) == 0 {
			return &types.StageResult{
				Name:   "store",
				Output: map[string]any{"stored_count": 0},
			}, nil
		}

		vectorSize := docChunks[0].Embedding.Dimensions
		collectionName := job.Args.Tag

		qdrantURL := job.Args.QdrantURL
		if qdrantURL == "" {
			qdrantURL = "http://localhost:6334"
		}
		qdrantAPIKey := job.Args.QdrantAPIKey
		if qdrantAPIKey == "" {
			qdrantAPIKey = os.Getenv("QDRANT_API_KEY")
		}

		qStore := qstore.NewQdrantStore(qdrantAPIKey)
		if err := qStore.Connect(ctx, qdrantURL); err != nil {
			return nil, fmt.Errorf("connect qdrant: %w", err)
		}
		defer qStore.Close()

		if err := qStore.EnsureCollection(ctx, collectionName, vectorSize, "Cosine"); err != nil {
			return nil, fmt.Errorf("ensure collection: %w", err)
		}
		if err := qStore.Store(ctx, docChunks); err != nil {
			return nil, fmt.Errorf("store chunks: %w", err)
		}

		return &types.StageResult{
			Name:   "store",
			Output: map[string]any{"stored_count": len(docChunks), "collection": collectionName},
		}, nil
	}); err != nil {
		return err
	}

	return w.Store.UpdateWorkflowStatus(ctx, job.Args.WorkflowID, "succeeded")
}
