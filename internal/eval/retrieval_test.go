package eval

import (
	"context"
	"testing"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type mockRetriever struct {
	mock.Mock
}

func (m *mockRetriever) Retrieve(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error) {
	args := m.Called(ctx, collection, query, topK)
	return args.Get(0).([]types.SearchResult), args.Error(1)
}

type mockGenerator struct {
	mock.Mock
}

func (m *mockGenerator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*openai.ChatCompletion), args.Error(1)
}

func TestNewRetrievalEvaluator(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 5)
	require.NotNil(t, e)
	assert.Equal(t, 5, e.topK)
	assert.Equal(t, r, e.retriever)
	assert.Equal(t, g, e.generator)
}

func TestEvaluate_SingleQuestion_Hit(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{
			ID:       "q1",
			Question: "test query",
			Relevance: []types.RelevanceJudgment{
				{DocumentID: "doc1.md", Grade: 3},
			},
		},
	}

	r.On("Retrieve", mock.Anything, "my-collection", "test query", 3).
		Return([]types.SearchResult{
			{DocumentPath: "doc1.md", Score: 0.95, Content: "relevant content"},
			{DocumentPath: "doc2.md", Score: 0.85},
			{DocumentPath: "doc3.md", Score: 0.75},
		}, nil)

	g.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "the answer",
					},
				},
			},
			Usage: openai.CompletionUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
			},
		}, nil)

	results, err := e.Evaluate(ctx, "my-collection", questions, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "q1", results[0].QuestionID)
	assert.Equal(t, "test query", results[0].Question)
	assert.True(t, results[0].Hit[1])
	assert.True(t, results[0].Hit[3])
	assert.Equal(t, 1, results[0].RankFirst)
	assert.Equal(t, "the answer", results[0].Answer)
	assert.Equal(t, 10, results[0].PromptTokens)
	assert.Equal(t, 20, results[0].CompletionTokens)
	assert.GreaterOrEqual(t, results[0].LatencyMs, int64(0))
}

func TestEvaluate_SingleQuestion_Miss(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{
			ID:       "q1",
			Question: "test query",
			Relevance: []types.RelevanceJudgment{
				{DocumentID: "docX.md", Grade: 3},
			},
		},
	}

	r.On("Retrieve", mock.Anything, "col", "test query", 3).
		Return([]types.SearchResult{
			{DocumentPath: "doc1.md", Score: 0.9},
			{DocumentPath: "doc2.md", Score: 0.8},
		}, nil)

	g.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "answer"}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 5, CompletionTokens: 10},
		}, nil)

	results, err := e.Evaluate(ctx, "col", questions, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.False(t, results[0].Hit[1])
	assert.False(t, results[0].Hit[3])
	assert.Equal(t, 0, results[0].RankFirst)
}

func TestEvaluate_MultipleQuestions_Ordering(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "first", Relevance: []types.RelevanceJudgment{{DocumentID: "d1.md", Grade: 1}}},
		{ID: "q2", Question: "second", Relevance: []types.RelevanceJudgment{{DocumentID: "d2.md", Grade: 1}}},
		{ID: "q3", Question: "third", Relevance: []types.RelevanceJudgment{{DocumentID: "d3.md", Grade: 1}}},
	}

	r.On("Retrieve", mock.Anything, "col", "first", 3).
		Return([]types.SearchResult{{DocumentPath: "d1.md", Score: 0.9, Content: "x"}}, nil)
	r.On("Retrieve", mock.Anything, "col", "second", 3).
		Return([]types.SearchResult{{DocumentPath: "d2.md", Score: 0.9, Content: "x"}}, nil)
	r.On("Retrieve", mock.Anything, "col", "third", 3).
		Return([]types.SearchResult{{DocumentPath: "d3.md", Score: 0.9, Content: "x"}}, nil)

	g.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "ans"}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 1, CompletionTokens: 1},
		}, nil)

	results, err := e.Evaluate(ctx, "col", questions, 2)
	require.NoError(t, err)
	require.Len(t, results, 3)

	assert.Equal(t, "q1", results[0].QuestionID)
	assert.Equal(t, "q2", results[1].QuestionID)
	assert.Equal(t, "q3", results[2].QuestionID)
}

func TestEvaluate_Concurrent_AllProcessed(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 3)

	ctx := context.Background()
	questions := make([]types.EvalQuestion, 4)
	for i := range questions {
		questions[i] = types.EvalQuestion{
			ID:       string(rune('a' + i)),
			Question: "query",
			Relevance: []types.RelevanceJudgment{
				{DocumentID: "doc.md", Grade: 1},
			},
		}
	}

	r.On("Retrieve", mock.Anything, "col", "query", 3).
		Return([]types.SearchResult{{DocumentPath: "doc.md", Score: 0.9, Content: "x"}}, nil)

	g.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "ans"}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 1, CompletionTokens: 1},
		}, nil)

	results, err := e.Evaluate(ctx, "col", questions, 2)
	require.NoError(t, err)
	assert.Len(t, results, 4)
	for _, r := range results {
		assert.True(t, r.Hit[1], "all questions should hit")
	}
}

func TestEvaluate_RetrieverError(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentID: "doc.md", Grade: 1}}},
	}

	r.On("Retrieve", mock.Anything, "col", "query", 3).
		Return([]types.SearchResult{}, assert.AnError)

	_, err := e.Evaluate(ctx, "col", questions, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evaluate question q1")
}

func TestEvaluate_GeneratorError_NonFatal(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentID: "doc.md", Grade: 1}}},
	}

	r.On("Retrieve", mock.Anything, "col", "query", 3).
		Return([]types.SearchResult{{DocumentPath: "doc.md", Score: 0.9, Content: "x"}}, nil)

	g.On("Generate", mock.Anything, mock.Anything).
		Return(nil, assert.AnError)

	results, err := e.Evaluate(ctx, "col", questions, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Hit[1])
	assert.Empty(t, results[0].Answer, "answer should be empty when generator fails")
}

func TestEvaluate_EmptyResults_MapsHitCorrectly(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 5)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentID: "doc.md", Grade: 1}}},
	}

	r.On("Retrieve", mock.Anything, "col", "query", 5).
		Return([]types.SearchResult{}, nil)

	results, err := e.Evaluate(ctx, "col", questions, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.False(t, results[0].Hit[1])
	assert.Equal(t, 0, results[0].RankFirst)
	assert.Empty(t, results[0].Answer)
}

func TestEvaluate_ZeroConcurrency_Defaults(t *testing.T) {
	r := new(mockRetriever)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "q", Relevance: []types.RelevanceJudgment{{DocumentID: "d.md", Grade: 1}}},
	}

	r.On("Retrieve", mock.Anything, "col", "q", 3).
		Return([]types.SearchResult{{DocumentPath: "d.md", Score: 0.9, Content: "x"}}, nil)
	g.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "a"}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 1, CompletionTokens: 1},
		}, nil)

	results, err := e.Evaluate(ctx, "col", questions, 0)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}
