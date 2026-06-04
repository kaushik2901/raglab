package eval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/retriever"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type RetrievalEvaluator struct {
	retriever *retriever.Retriever
	generator *generator.Generator
	topK      int
}

func NewRetrievalEvaluator(ret *retriever.Retriever, gen *generator.Generator, topK int) *RetrievalEvaluator {
	return &RetrievalEvaluator{
		retriever: ret,
		generator: gen,
		topK:      topK,
	}
}

func (e *RetrievalEvaluator) Evaluate(ctx context.Context, collection string, questions []types.EvalQuestion) ([]types.RetrievalResult, error) {
	results := make([]types.RetrievalResult, 0, len(questions))

	for _, q := range questions {
		result, err := e.evaluateOne(ctx, collection, q)
		if err != nil {
			return nil, fmt.Errorf("evaluate question %s: %w", q.ID, err)
		}
		results = append(results, *result)
	}

	return results, nil
}

func (e *RetrievalEvaluator) evaluateOne(ctx context.Context, collection string, q types.EvalQuestion) (*types.RetrievalResult, error) {
	start := time.Now()

	searchResults, err := e.retriever.Retrieve(ctx, collection, q.Question, e.topK)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}

	result := &types.RetrievalResult{
		QuestionID:    q.ID,
		Question:      q.Question,
		ExpectedPaths: q.SourcePaths,
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
		for i := 0; i < min(k, len(searchResults)); i++ {
			if containsPath(q.SourcePaths, searchResults[i].DocumentPath) {
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

	if e.generator != nil {
		var contextParts []string
		for _, sr := range searchResults {
			contextParts = append(contextParts, fmt.Sprintf("Document: %s\nContent:\n%s", sr.DocumentPath, sr.Content))
		}
		contextText := strings.Join(contextParts, "\n\n---\n\n")

		systemPrompt := "You are a helpful assistant that answers questions based solely on the provided context. If the context does not contain enough information to answer, say so."
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
		}
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result, nil
}
