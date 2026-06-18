package eval

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/openai/openai-go"
	"golang.org/x/sync/errgroup"

	"github.com/kaushik2901/raglab/internal/embedder"
	"github.com/kaushik2901/raglab/internal/generator"
	"github.com/kaushik2901/raglab/internal/types"
)

type Retriever interface {
	Retrieve(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error)
}

type RetrievalEvaluator struct {
	retriever Retriever
	embedder  embedder.Embedder
	generator generator.Generator
	judge     generator.Generator
	topK      int
}

func NewRetrievalEvaluator(ret Retriever, emb embedder.Embedder, gen generator.Generator, topK int) *RetrievalEvaluator {
	return &RetrievalEvaluator{
		retriever: ret,
		embedder:  emb,
		generator: gen,
		topK:      topK,
	}
}

func NewRetrievalEvaluatorWithJudge(ret Retriever, emb embedder.Embedder, gen generator.Generator, judge generator.Generator, topK int) *RetrievalEvaluator {
	return &RetrievalEvaluator{
		retriever: ret,
		embedder:  emb,
		generator: gen,
		judge:     judge,
		topK:      topK,
	}
}

func (e *RetrievalEvaluator) Evaluate(ctx context.Context, collection string, questions []types.EvalQuestion, concurrency int) ([]types.RetrievalResult, error) {
	if concurrency <= 0 {
		concurrency = 5
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	results := make([]types.RetrievalResult, len(questions))
	for i, q := range questions {
		i, q := i, q
		g.Go(func() error {
			result, err := e.evaluateOne(ctx, collection, q)
			if err != nil {
				return fmt.Errorf("evaluate question %s: %w", q.ID, err)
			}
			results[i] = *result
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

func (e *RetrievalEvaluator) evaluateOne(ctx context.Context, collection string, q types.EvalQuestion) (*types.RetrievalResult, error) {
	start := time.Now()

	queryChunk := types.Chunk{ID: q.ID, Content: q.Question}
	embeddings, err := e.embedder.Embed(ctx, []types.Chunk{queryChunk})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	queryVector := ToFloat32(embeddings[0].Vector)

	searchResults, err := e.retriever.Retrieve(ctx, collection, queryVector, e.topK)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}

	expectedPaths := make([]string, 0, len(q.Relevance))
	for _, j := range q.Relevance {
		if j.Grade > 0 {
			expectedPaths = append(expectedPaths, j.DocumentPath)
		}
	}

	result := &types.RetrievalResult{
		QuestionID:     q.ID,
		Question:       q.Question,
		Category:       q.Category,
		Difficulty:     q.Difficulty,
		ExpectedAnswer: q.ExpectedAnswer,
		Relevance:      q.Relevance,
		ExpectedPaths:  expectedPaths,
	}

	if len(searchResults) == 0 {
		result.Hit = map[int]bool{}
		for k := 1; k <= e.topK; k++ {
			result.Hit[k] = false
		}
		result.RankFirst = 0
		return result, nil
	}

	result.RetrievedPaths = make([]string, len(searchResults))
	result.Scores = make([]float64, len(searchResults))
	for i, sr := range searchResults {
		result.RetrievedPaths[i] = sr.DocumentPath
		result.Scores[i] = float64(sr.Score)
	}

	foundFirst := false
	result.Hit = make(map[int]bool)
	for k := 1; k <= e.topK; k++ {
		hit := false
		for i := range min(k, len(searchResults)) {
			if containsPath(expectedPaths, searchResults[i].DocumentPath) {
				hit = true
				if !foundFirst {
					result.RankFirst = i + 1
					foundFirst = true
				}
				break
			}
		}
		result.Hit[k] = hit
	}
	if !foundFirst {
		result.RankFirst = 0
	}

	// Per-question graded NDCG
	result.NDCGGraded = computeNDCGGradedForQuestion(q.Relevance, result.RetrievedPaths, e.topK)

	if e.generator != nil {
		contextText := buildContextText(searchResults)

		systemPrompt := SystemPrompt
		userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s\n\nAnswer the question based on the context above.", contextText, q.Question)

		completion, err := e.generator.Generate(ctx, openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(systemPrompt),
				openai.UserMessage(userPrompt),
			},
			Temperature: openai.Float(0.3),
			MaxTokens:   openai.Int(1024),
		})
		if err == nil && len(completion.Choices) > 0 {
			result.Answer = completion.Choices[0].Message.Content
			usage := completion.Usage
			result.PromptTokens = int(usage.PromptTokens)
			result.CompletionTokens = int(usage.CompletionTokens)

			if e.judge != nil {
				score, judgeErr := JudgeAnswer(ctx, e.judge, q.Question, contextText, q.ExpectedAnswer, result.Answer)
				if judgeErr != nil {
					slog.Warn("judge error", "question_id", q.ID, "err", judgeErr)
				} else {
					result.AnswerScore = score
				}
			}
		}
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result, nil
}
