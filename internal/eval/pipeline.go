package eval

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

const SystemPrompt = "You are a helpful assistant that answers questions based solely on the provided context. If the context does not contain enough information to answer, say so."

type VectorSearcher interface {
	Search(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error)
}

type PipelineArgs struct {
	Embedder    embedder.Embedder
	Searcher    VectorSearcher
	Generator   generator.Generator
	JudgeGen    generator.Generator
	Collection  string
	Questions   []types.EvalQuestion
	TopK        int
	EmbedBatch  int
	Concurrency int
}

func Evaluate(ctx context.Context, args PipelineArgs) ([]types.RetrievalResult, error) {
	if args.Concurrency > 0 {
		slog.Debug("concurrency requested but eval runs sequentially per design", "requested", args.Concurrency)
	}
	slog.Info("phase 1: batch embedding queries",
		"count", len(args.Questions), "batch_size", args.EmbedBatch)
	embeddings, err := batchEmbedQueries(ctx, args.Embedder, args.Questions, args.EmbedBatch)
	if err != nil {
		return nil, fmt.Errorf("phase 1 (embed): %w", err)
	}

	slog.Info("phase 2: sequential search + generate + judge", "count", len(args.Questions))
	results := make([]types.RetrievalResult, len(args.Questions))
	overallStart := time.Now()

	for i, q := range args.Questions {
		qStart := time.Now()

		qCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)

		queryVector := toFloat32(embeddings[i])
		searchResults, err := args.Searcher.Search(qCtx, args.Collection, queryVector, args.TopK)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("search q=%s: %w", q.ID, err)
		}

		fillRetrievalResult(&results[i], q, searchResults, args.TopK)

		if args.Generator != nil && len(searchResults) > 0 {
			contextText := buildContextText(searchResults)
			answer, promptTokens, completionTokens := generateForQuestion(qCtx, args.Generator, q, contextText)
			results[i].Answer = answer
			results[i].PromptTokens = promptTokens
			results[i].CompletionTokens = completionTokens

			if answer == "" {
				results[i].Failed = true
			}

			if args.JudgeGen != nil && answer != "" {
				score, err := JudgeAnswer(qCtx, args.JudgeGen, q.Question, contextText, q.ExpectedAnswer, answer)
				if err != nil {
					slog.Warn("judge error", "question_id", q.ID, "err", err)
					results[i].Failed = true
				} else {
					results[i].AnswerScore = score
				}
			}
		}

		results[i].LatencyMs = time.Since(qStart).Milliseconds()
		cancel()
	}

	slog.Info("evaluation complete",
		"total", len(results),
		"duration", time.Since(overallStart).Round(time.Second))

	return results, nil
}

func batchEmbedQueries(ctx context.Context, emb embedder.Embedder, questions []types.EvalQuestion, batchSize int) ([][]float64, error) {
	if len(questions) == 0 {
		return nil, nil
	}

	var allEmbeddings [][]float64
	for i := 0; i < len(questions); i += batchSize {
		end := i + batchSize
		if end > len(questions) {
			end = len(questions)
		}

		chunks := make([]types.Chunk, 0, end-i)
		for _, q := range questions[i:end] {
			chunks = append(chunks, types.Chunk{ID: q.ID, Content: q.Question})
		}

		embeddings, err := emb.Embed(ctx, chunks)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}

		for _, e := range embeddings {
			allEmbeddings = append(allEmbeddings, e.Vector)
		}
	}

	return allEmbeddings, nil
}


