package eval

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openai/openai-go"

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
		defer cancel()

		queryVector := toFloat32(embeddings[i])
		searchResults, err := args.Searcher.Search(qCtx, args.Collection, queryVector, args.TopK)
		if err != nil {
			return nil, fmt.Errorf("search q=%s: %w", q.ID, err)
		}

		fillRetrievalResult(&results[i], q, searchResults, args.TopK)

		if args.Generator != nil && len(searchResults) > 0 {
			contextText := buildContextText(searchResults)
			answer, promptTokens, completionTokens := generateForQuestion(qCtx, args.Generator, q, contextText)
			results[i].Answer = answer
			results[i].PromptTokens = promptTokens
			results[i].CompletionTokens = completionTokens

			if args.JudgeGen != nil && answer != "" {
				score, err := JudgeAnswer(qCtx, args.JudgeGen, q.Question, contextText, q.ExpectedAnswer, answer)
				if err != nil {
					slog.Warn("judge error", "question_id", q.ID, "err", err)
				} else {
					results[i].AnswerScore = score
				}
			}
		}

		results[i].LatencyMs = time.Since(qStart).Milliseconds()
	}

	slog.Info("evaluation complete",
		"total", len(results),
		"duration", time.Since(overallStart).Round(time.Second))

	return results, nil
}

func toFloat32(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = float32(f)
	}
	return out
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

func fillRetrievalResult(r *types.RetrievalResult, q types.EvalQuestion, searchResults []types.SearchResult, topK int) {
	r.QuestionID = q.ID
	r.Question = q.Question
	r.Category = q.Category
	r.Difficulty = q.Difficulty
	r.ExpectedAnswer = q.ExpectedAnswer
	r.Relevance = q.Relevance

	for _, j := range q.Relevance {
		if j.Grade > 0 {
			r.ExpectedPaths = append(r.ExpectedPaths, j.DocumentPath)
		}
	}

	for _, sr := range searchResults {
		r.RetrievedPaths = append(r.RetrievedPaths, sr.DocumentPath)
		r.Scores = append(r.Scores, float64(sr.Score))
	}

	foundFirst := false
	r.Hit = make(map[int]bool)
	for k := 1; k <= topK; k++ {
		hit := false
		for i := 0; i < min(k, len(searchResults)); i++ {
			if containsPath(r.ExpectedPaths, searchResults[i].DocumentPath) {
				hit = true
				if !foundFirst {
					r.RankFirst = i + 1
					foundFirst = true
				}
				break
			}
		}
		r.Hit[k] = hit
	}
	if !foundFirst {
		r.RankFirst = 0
	}

	r.NDCGGraded = computeNDCGGradedForQuestion(q.Relevance, r.RetrievedPaths, topK)
}

func buildContextText(searchResults []types.SearchResult) string {
	var parts []string
	for _, sr := range searchResults {
		parts = append(parts, fmt.Sprintf("Document: %s\nContent:\n%s", sr.DocumentPath, sr.Content))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func generateForQuestion(ctx context.Context, gen generator.Generator, q types.EvalQuestion, contextText string) (string, int, int) {
	systemPrompt := SystemPrompt
	userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s\n\nAnswer the question based on the context above.", contextText, q.Question)

	completion, err := gen.Generate(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Temperature: openai.Float(0.3),
		MaxTokens:   openai.Int(1024),
	})
	if err != nil {
		slog.Warn("generate error", "question_id", q.ID, "err", err)
		return "", 0, 0
	}
	if len(completion.Choices) == 0 {
		return "", 0, 0
	}
	return completion.Choices[0].Message.Content,
		int(completion.Usage.PromptTokens),
		int(completion.Usage.CompletionTokens)
}
