package eval

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openai/openai-go"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func EvaluateQuestion(
	ctx context.Context,
	q types.EvalQuestion,
	embedding []float64,
	searcher VectorSearcher,
	gen generator.Generator,
	judgeGen generator.Generator,
	collection string,
	topK int,
) (types.RetrievalResult, error) {
	qStart := time.Now()

	qCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	queryVector := toFloat32(embedding)
	searchResults, err := searcher.Search(qCtx, collection, queryVector, topK)
	if err != nil {
		return types.RetrievalResult{}, fmt.Errorf("search q=%s: %w", q.ID, err)
	}

	var result types.RetrievalResult
	fillRetrievalResult(&result, q, searchResults, topK)

	if gen != nil && len(searchResults) > 0 {
		contextText := buildContextText(searchResults)
		answer, promptTokens, completionTokens := generateForQuestion(qCtx, gen, q, contextText)
		result.Answer = answer
		result.PromptTokens = promptTokens
		result.CompletionTokens = completionTokens

		if answer == "" {
			result.Failed = true
		}

		if judgeGen != nil && answer != "" {
			score, err := JudgeAnswer(qCtx, judgeGen, q.Question, contextText, q.ExpectedAnswer, answer)
			if err != nil {
				slog.Warn("judge error", "question_id", q.ID, "err", err)
				result.Failed = true
			} else {
				result.AnswerScore = score
			}
		}
	}

	result.LatencyMs = time.Since(qStart).Milliseconds()
	return result, nil
}

func toFloat32(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = float32(f)
	}
	return out
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
		for i := range min(k, len(searchResults)) {
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
	r.NDCGGradedK = topK
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
