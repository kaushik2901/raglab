package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockVectorSearcher struct {
	searchFn func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error)
}

func (m *mockVectorSearcher) Search(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	return m.searchFn(ctx, collection, queryVector, topK)
}

type mockGen struct {
	generateFn      func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	generateStreamFn func(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error)
	modelNameFn     func() string
}

func (m *mockGen) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return m.generateFn(ctx, params)
}

func (m *mockGen) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error) {
	return m.generateStreamFn(ctx, params, cb)
}

func (m *mockGen) ModelName() string {
	if m.modelNameFn != nil {
		return m.modelNameFn()
	}
	return "mock"
}

func TestToFloat32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []float64
		want  []float32
	}{
		{"nil input", nil, []float32{}},
		{"empty slice", []float64{}, []float32{}},
		{"single value", []float64{3.14}, []float32{3.14}},
		{"multiple values", []float64{1.1, 2.2, 3.3}, []float32{1.1, 2.2, 3.3}},
		{"integers", []float64{1, 2, 3}, []float32{1, 2, 3}},
		{"rounding", []float64{0.3333333333}, []float32{0.33333334}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toFloat32(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildContextText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		results  []types.SearchResult
		contains []string
	}{
		{
			name:     "empty",
			results:  nil,
			contains: []string{},
		},
		{
			name: "single doc",
			results: []types.SearchResult{
				{DocumentPath: "doc1.md", Content: "content one"},
			},
			contains: []string{"Document: doc1.md", "content one"},
		},
		{
			name: "multiple docs",
			results: []types.SearchResult{
				{DocumentPath: "a.md", Content: "alpha"},
				{DocumentPath: "b.md", Content: "beta"},
			},
			contains: []string{"Document: a.md", "alpha", "Document: b.md", "beta", "---"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildContextText(tt.results)
			for _, s := range tt.contains {
				assert.Contains(t, got, s)
			}
		})
	}
}

func TestFillRetrievalResult_HitAtRank1(t *testing.T) {
	q := types.EvalQuestion{
		ID:       "q1",
		Question: "test query",
		Category: "cat-a",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc1.md", Grade: 3},
		},
	}
	results := []types.SearchResult{
		{DocumentPath: "doc1.md", Score: 0.95, Content: "relevant"},
		{DocumentPath: "doc2.md", Score: 0.80},
		{DocumentPath: "doc3.md", Score: 0.70},
	}

	var r types.RetrievalResult
	fillRetrievalResult(&r, q, results, 3)

	assert.Equal(t, "q1", r.QuestionID)
	assert.Equal(t, "test query", r.Question)
	assert.Equal(t, "cat-a", r.Category)
	assert.Equal(t, []string{"doc1.md"}, r.ExpectedPaths)
	assert.Equal(t, []string{"doc1.md", "doc2.md", "doc3.md"}, r.RetrievedPaths)
	assert.True(t, r.Hit[1])
	assert.True(t, r.Hit[3])
	assert.Equal(t, 1, r.RankFirst)
}

func TestFillRetrievalResult_HitAtRank3(t *testing.T) {
	q := types.EvalQuestion{
		ID: "q1",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "docX.md", Grade: 3},
		},
	}
	results := []types.SearchResult{
		{DocumentPath: "docA.md", Score: 0.95},
		{DocumentPath: "docB.md", Score: 0.85},
		{DocumentPath: "docX.md", Score: 0.75},
	}

	var r types.RetrievalResult
	fillRetrievalResult(&r, q, results, 5)

	assert.False(t, r.Hit[1])
	assert.False(t, r.Hit[2])
	assert.True(t, r.Hit[3])
	assert.True(t, r.Hit[5])
	assert.Equal(t, 3, r.RankFirst)
}

func TestFillRetrievalResult_Miss(t *testing.T) {
	q := types.EvalQuestion{
		ID: "q1",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "missing.md", Grade: 3},
		},
	}
	results := []types.SearchResult{
		{DocumentPath: "doc1.md", Score: 0.9},
		{DocumentPath: "doc2.md", Score: 0.8},
	}

	var r types.RetrievalResult
	fillRetrievalResult(&r, q, results, 3)

	assert.False(t, r.Hit[1])
	assert.False(t, r.Hit[3])
	assert.Equal(t, 0, r.RankFirst)
}

func TestFillRetrievalResult_EmptyResults(t *testing.T) {
	q := types.EvalQuestion{
		ID: "q1",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc1.md", Grade: 3},
		},
	}

	var r types.RetrievalResult
	fillRetrievalResult(&r, q, nil, 3)

	assert.False(t, r.Hit[1])
	assert.False(t, r.Hit[3])
	assert.Equal(t, 0, r.RankFirst)
	assert.Empty(t, r.RetrievedPaths)
	assert.Empty(t, r.Scores)
}

func TestFillRetrievalResult_MultipleExpectedPaths(t *testing.T) {
	q := types.EvalQuestion{
		ID: "q1",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc1.md", Grade: 3},
			{DocumentPath: "doc3.md", Grade: 2},
			{DocumentPath: "docX.md", Grade: 0},
		},
	}
	results := []types.SearchResult{
		{DocumentPath: "doc1.md", Score: 0.9},
		{DocumentPath: "doc2.md", Score: 0.8},
		{DocumentPath: "doc3.md", Score: 0.7},
	}

	var r types.RetrievalResult
	fillRetrievalResult(&r, q, results, 5)

	assert.Equal(t, []string{"doc1.md", "doc3.md"}, r.ExpectedPaths)
	assert.True(t, r.Hit[1])
	assert.True(t, r.Hit[3])
	assert.Equal(t, 1, r.RankFirst)
}

func TestGenerateForQuestion_Success(t *testing.T) {
	gen := &mockGen{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: "the answer"}},
				},
				Usage: openai.CompletionUsage{PromptTokens: 10, CompletionTokens: 20},
			}, nil
		},
	}

	answer, prompt, completion := generateForQuestion(context.Background(), gen, types.EvalQuestion{Question: "q?"}, "context text")
	assert.Equal(t, "the answer", answer)
	assert.Equal(t, 10, prompt)
	assert.Equal(t, 20, completion)
}

func TestGenerateForQuestion_APIError(t *testing.T) {
	gen := &mockGen{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, errors.New("api error")
		},
	}

	answer, prompt, completion := generateForQuestion(context.Background(), gen, types.EvalQuestion{Question: "q?"}, "ctx")
	assert.Empty(t, answer)
	assert.Equal(t, 0, prompt)
	assert.Equal(t, 0, completion)
}

func TestGenerateForQuestion_EmptyChoices(t *testing.T) {
	gen := &mockGen{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{},
			}, nil
		},
	}

	answer, _, _ := generateForQuestion(context.Background(), gen, types.EvalQuestion{Question: "q?"}, "ctx")
	assert.Empty(t, answer)
}

func TestEvaluateQuestion_Hit(t *testing.T) {
	searcher := &mockVectorSearcher{
		searchFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return []types.SearchResult{
				{DocumentPath: "doc1.md", Score: 0.95, Content: "relevant content"},
			}, nil
		},
	}
	gen := &mockGen{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: "answer"}},
				},
				Usage: openai.CompletionUsage{PromptTokens: 5, CompletionTokens: 10},
			}, nil
		},
	}

	q := types.EvalQuestion{
		ID:       "q1",
		Question: "test",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc1.md", Grade: 3},
		},
	}

	result, err := EvaluateQuestion(context.Background(), q, []float64{0.1, 0.2, 0.3}, searcher, gen, nil, "col", 5)
	require.NoError(t, err)
	assert.Equal(t, "q1", result.QuestionID)
	assert.True(t, result.Hit[1])
	assert.Equal(t, 1, result.RankFirst)
	assert.Equal(t, "answer", result.Answer)
	assert.Equal(t, 5, result.PromptTokens)
	assert.Equal(t, 10, result.CompletionTokens)
	assert.GreaterOrEqual(t, result.LatencyMs, int64(0))
	assert.False(t, result.Failed)
}

func TestEvaluateQuestion_Miss(t *testing.T) {
	searcher := &mockVectorSearcher{
		searchFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return []types.SearchResult{
				{DocumentPath: "docOther.md", Score: 0.9, Content: "irrelevant"},
			}, nil
		},
	}
	gen := &mockGen{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: "ans"}},
				},
				Usage: openai.CompletionUsage{PromptTokens: 1, CompletionTokens: 1},
			}, nil
		},
	}

	q := types.EvalQuestion{
		ID:       "q1",
		Question: "test",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "docTarget.md", Grade: 3},
		},
	}

	result, err := EvaluateQuestion(context.Background(), q, []float64{0.1}, searcher, gen, nil, "col", 5)
	require.NoError(t, err)
	assert.False(t, result.Hit[1])
	assert.Equal(t, 0, result.RankFirst)
}

func TestEvaluateQuestion_SearchError(t *testing.T) {
	searcher := &mockVectorSearcher{
		searchFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return nil, errors.New("search failed")
		},
	}

	q := types.EvalQuestion{ID: "q1", Question: "test"}
	_, err := EvaluateQuestion(context.Background(), q, []float64{0.1}, searcher, nil, nil, "col", 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search")
}

func TestEvaluateQuestion_WithJudge(t *testing.T) {
	searcher := &mockVectorSearcher{
		searchFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return []types.SearchResult{
				{DocumentPath: "doc1.md", Score: 0.95, Content: "content"},
			}, nil
		},
	}
	gen := &mockGen{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: "generated answer"}},
				},
				Usage: openai.CompletionUsage{PromptTokens: 5, CompletionTokens: 10},
			}, nil
		},
	}
	judge := &mockGen{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return &openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: `{"score": 0.85, "reasoning": "correct"}`}},
				},
				Usage: openai.CompletionUsage{PromptTokens: 2, CompletionTokens: 1},
			}, nil
		},
	}

	q := types.EvalQuestion{
		ID:             "q1",
		Question:       "test",
		ExpectedAnswer: "expected",
		Relevance:      []types.RelevanceJudgment{{DocumentPath: "doc1.md", Grade: 3}},
	}

	result, err := EvaluateQuestion(context.Background(), q, []float64{0.1}, searcher, gen, judge, "col", 5)
	require.NoError(t, err)
	assert.InDelta(t, 0.85, result.AnswerScore, 0.001)
	assert.False(t, result.Failed)
}

func TestEvaluateQuestion_GenErrorNonFatal(t *testing.T) {
	searcher := &mockVectorSearcher{
		searchFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return []types.SearchResult{
				{DocumentPath: "doc1.md", Score: 0.9, Content: "content"},
			}, nil
		},
	}
	gen := &mockGen{
		generateFn: func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			return nil, errors.New("gen error")
		},
	}

	q := types.EvalQuestion{
		ID:       "q1",
		Question: "test",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc1.md", Grade: 3},
		},
	}

	result, err := EvaluateQuestion(context.Background(), q, []float64{0.1}, searcher, gen, nil, "col", 5)
	require.NoError(t, err)
	assert.True(t, result.Hit[1])
	assert.Empty(t, result.Answer)
	assert.True(t, result.Failed)
}

func TestEvaluateQuestion_NoGenerator(t *testing.T) {
	searcher := &mockVectorSearcher{
		searchFn: func(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
			return []types.SearchResult{
				{DocumentPath: "doc1.md", Score: 0.9, Content: "content"},
			}, nil
		},
	}

	q := types.EvalQuestion{
		ID:       "q1",
		Question: "test",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc1.md", Grade: 3},
		},
	}

	result, err := EvaluateQuestion(context.Background(), q, []float64{0.1}, searcher, nil, nil, "col", 5)
	require.NoError(t, err)
	assert.True(t, result.Hit[1])
	assert.Empty(t, result.Answer, "no answer when generator is nil")
}
