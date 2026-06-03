package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	stagepkg "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/stage"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
	"github.com/riverqueue/river"
)

type ParseArgs struct {
	WorkflowID string `json:"workflow_id"`
	Tag        string `json:"tag"`
	InputTag   string `json:"input_tag"`
}

func (ParseArgs) Kind() string { return "parse" }

type ChunkArgs struct {
	WorkflowID    string `json:"workflow_id"`
	Tag           string `json:"tag"`
	InputTag      string `json:"input_tag"`
	ChunkStrategy string `json:"chunk_strategy"`
	ChunkSize     int    `json:"chunk_size"`
	ChunkOverlap  int    `json:"chunk_overlap"`
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
	stepID, err := w.Store.CreateStep(ctx, job.Args.WorkflowID, "parse")
	if err != nil {
		return fmt.Errorf("create step: %w", err)
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "running", nil, nil); err != nil {
		return fmt.Errorf("mark step running: %w", err)
	}

	inputPath := filepath.Join("artifacts", "preprocessing", job.Args.InputTag, "output")

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.ParseStage(inputPath)
	result, err := stage.Run(ctx, state)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return err
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "succeeded", nil, result.Output); err != nil {
		return fmt.Errorf("mark step succeeded: %w", err)
	}

	_, err = w.Client.Insert(ctx, &ChunkArgs{
		WorkflowID:    job.Args.WorkflowID,
		Tag:           job.Args.Tag,
		InputTag:      job.Args.InputTag,
		ChunkStrategy: "fixed",
		ChunkSize:     512,
		ChunkOverlap:  64,
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
	stepID, err := w.Store.CreateStep(ctx, job.Args.WorkflowID, "chunk")
	if err != nil {
		return fmt.Errorf("create step: %w", err)
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "running", nil, nil); err != nil {
		return fmt.Errorf("mark step running: %w", err)
	}

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.ChunkStage(job.Args.ChunkStrategy, job.Args.ChunkSize, job.Args.ChunkOverlap)
	result, err := stage.Run(ctx, state)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return err
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "succeeded", nil, result.Output); err != nil {
		return fmt.Errorf("mark step succeeded: %w", err)
	}

	_, err = w.Client.Insert(ctx, &EmbedArgs{
		WorkflowID:     job.Args.WorkflowID,
		Tag:            job.Args.Tag,
		InputTag:       job.Args.InputTag,
		EmbeddingModel: "text-embedding-3-small",
		BatchSize:      20,
		LLMBaseURL:     "https://api.openai.com/v1",
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
	stepID, err := w.Store.CreateStep(ctx, job.Args.WorkflowID, "embed")
	if err != nil {
		return fmt.Errorf("create step: %w", err)
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "running", nil, nil); err != nil {
		return fmt.Errorf("mark step running: %w", err)
	}

	apiKey := job.Args.LLMApiKey
	if apiKey == "" {
		apiKey = os.Getenv("LLM_API_KEY")
	}

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	stage := stagepkg.EmbedStage(job.Args.LLMBaseURL, apiKey, job.Args.EmbeddingModel, job.Args.BatchSize)
	result, err := stage.Run(ctx, state)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return err
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "succeeded", nil, result.Output); err != nil {
		return fmt.Errorf("mark step succeeded: %w", err)
	}

	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
	_, err = w.Client.Insert(ctx, &StoreArgs{
		WorkflowID:   job.Args.WorkflowID,
		Tag:          job.Args.Tag,
		InputTag:     job.Args.InputTag,
		QdrantURL:    "http://localhost:6334",
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
	stepID, err := w.Store.CreateStep(ctx, job.Args.WorkflowID, "store")
	if err != nil {
		return fmt.Errorf("create step: %w", err)
	}

	if err := w.Store.UpdateStepStatus(ctx, stepID, "running", nil, nil); err != nil {
		return fmt.Errorf("mark step running: %w", err)
	}

	state, err := w.Store.LoadState(ctx, job.Args.WorkflowID)
	if err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("load state: %w", err)
	}

	docChunks, ok := state["document_chunks"].([]types.DocumentChunk)
	if !ok {
		errStr := "document_chunks not found in state"
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("%s", errStr)
	}

	if len(docChunks) == 0 {
		output := map[string]any{"stored_count": 0}
		if err := w.Store.UpdateStepStatus(ctx, stepID, "succeeded", nil, output); err != nil {
			return fmt.Errorf("mark step succeeded: %w", err)
		}
		if err := w.Store.UpdateWorkflowStatus(ctx, job.Args.WorkflowID, "succeeded"); err != nil {
			return fmt.Errorf("mark workflow succeeded: %w", err)
		}
		return nil
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
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("connect qdrant: %w", err)
	}
	defer qStore.Close()

	if err := qStore.EnsureCollection(ctx, collectionName, vectorSize, "Cosine"); err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("ensure collection: %w", err)
	}

	if err := qStore.Store(ctx, docChunks); err != nil {
		errStr := err.Error()
		w.Store.UpdateStepStatus(ctx, stepID, "failed", &errStr, nil)
		return fmt.Errorf("store chunks: %w", err)
	}

	output := map[string]any{"stored_count": len(docChunks), "collection": collectionName}
	if err := w.Store.UpdateStepStatus(ctx, stepID, "succeeded", nil, output); err != nil {
		return fmt.Errorf("mark step succeeded: %w", err)
	}

	if err := w.Store.UpdateWorkflowStatus(ctx, job.Args.WorkflowID, "succeeded"); err != nil {
		return fmt.Errorf("mark workflow succeeded: %w", err)
	}

	return nil
}

func RunParseStep(ctx context.Context, args ParseArgs, state map[string]any) (*types.StageResult, error) {
	stage := stagepkg.ParseStage(filepath.Join("artifacts", "preprocessing", args.InputTag, "output"))
	return stage.Run(ctx, state)
}

func RunChunkStep(ctx context.Context, args ChunkArgs, state map[string]any) (*types.StageResult, error) {
	stage := stagepkg.ChunkStage(args.ChunkStrategy, args.ChunkSize, args.ChunkOverlap)
	return stage.Run(ctx, state)
}

func RunEmbedStep(ctx context.Context, args EmbedArgs, state map[string]any) (*types.StageResult, error) {
	apiKey := args.LLMApiKey
	if apiKey == "" {
		apiKey = os.Getenv("LLM_API_KEY")
	}
	stage := stagepkg.EmbedStage(args.LLMBaseURL, apiKey, args.EmbeddingModel, args.BatchSize)
	return stage.Run(ctx, state)
}

func RunStoreStep(ctx context.Context, args StoreArgs, state map[string]any) (*types.StageResult, error) {
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

	qdrantURL := args.QdrantURL
	if qdrantURL == "" {
		qdrantURL = "http://localhost:6334"
	}

	qdrantAPIKey := args.QdrantAPIKey
	if qdrantAPIKey == "" {
		qdrantAPIKey = os.Getenv("QDRANT_API_KEY")
	}

	qStore := qstore.NewQdrantStore(qdrantAPIKey)
	if err := qStore.Connect(ctx, qdrantURL); err != nil {
		return nil, fmt.Errorf("connect qdrant: %w", err)
	}
	defer qStore.Close()

	vectorSize := docChunks[0].Embedding.Dimensions
	collectionName := args.Tag

	if err := qStore.EnsureCollection(ctx, collectionName, vectorSize, "Cosine"); err != nil {
		return nil, fmt.Errorf("ensure collection: %w", err)
	}
	if err := qStore.Store(ctx, docChunks); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}

	return &types.StageResult{
		Name:   "store",
		Output: map[string]any{"stored_count": len(docChunks), "collection": collectionName},
	}, nil
}
