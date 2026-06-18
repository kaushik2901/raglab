package eval

import (
	"context"
	"testing"

	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/raglab/internal/generator"
	"github.com/kaushik2901/raglab/internal/types"
)

type mockRetriever struct {
	mock.Mock
}

func (m *mockRetriever) Retrieve(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	args := m.Called(ctx, collection, queryVector, topK)
	return args.Get(0).([]types.SearchResult), args.Error(1)
}

type mockEmbedder struct {
	mock.Mock
}

func (m *mockEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
	args := m.Called(ctx, chunks)
	return args.Get(0).([]types.Embedding), args.Error(1)
}

func (m *mockEmbedder) Dimensions() int {
	args := m.Called()
	return args.Int(0)
}

func (m *mockEmbedder) ModelName() string {
	args := m.Called()
	return args.String(0)
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

func (m *mockGenerator) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb generator.StreamCallback) (*openai.ChatCompletion, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*openai.ChatCompletion), args.Error(1)
}

func (m *mockGenerator) ModelName() string {
	args := m.Called()
	return args.String(0)
}

func TestNewRetrievalEvaluator(t *testing.T) {
	r := new(mockRetriever)
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 5)
	require.NotNil(t, e)
	assert.Equal(t, 5, e.topK)
	assert.Equal(t, r, e.retriever)
	assert.Equal(t, g, e.generator)
}

func TestNewRetrievalEvaluatorWithJudge(t *testing.T) {
	r := new(mockRetriever)
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	j := new(mockGenerator)
	e := NewRetrievalEvaluatorWithJudge(r, emb, g, j, 5)
	require.NotNil(t, e)
	assert.Equal(t, 5, e.topK)
	assert.Equal(t, j, e.judge)
}

func TestEvaluate_SingleQuestion_Hit(t *testing.T) {
	r := new(mockRetriever)
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{
			ID:       "q1",
			Question: "test query",
			Relevance: []types.RelevanceJudgment{
				{DocumentPath: "doc1.md", Grade: 3},
			},
		},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1, 0.2}}}, nil)

	r.On("Retrieve", mock.Anything, "my-collection", mock.Anything, 3).
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
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{
			ID:       "q1",
			Question: "test query",
			Relevance: []types.RelevanceJudgment{
				{DocumentPath: "docX.md", Grade: 3},
			},
		},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 3).
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
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "first", Relevance: []types.RelevanceJudgment{{DocumentPath: "d1.md", Grade: 1}}},
		{ID: "q2", Question: "second", Relevance: []types.RelevanceJudgment{{DocumentPath: "d2.md", Grade: 1}}},
		{ID: "q3", Question: "third", Relevance: []types.RelevanceJudgment{{DocumentPath: "d3.md", Grade: 1}}},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 3).
		Return([]types.SearchResult{{DocumentPath: "d1.md", Score: 0.9, Content: "x"}}, nil)

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
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 3)

	ctx := context.Background()
	questions := make([]types.EvalQuestion, 4)
	for i := range questions {
		questions[i] = types.EvalQuestion{
			ID:       string(rune('a' + i)),
			Question: "query",
			Relevance: []types.RelevanceJudgment{
				{DocumentPath: "doc.md", Grade: 1},
			},
		}
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 3).
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
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentPath: "doc.md", Grade: 1}}},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 3).
		Return([]types.SearchResult{}, assert.AnError)

	_, err := e.Evaluate(ctx, "col", questions, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evaluate question q1")
}

func TestEvaluate_GeneratorError_NonFatal(t *testing.T) {
	r := new(mockRetriever)
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentPath: "doc.md", Grade: 1}}},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 3).
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
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 5)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentPath: "doc.md", Grade: 1}}},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 5).
		Return([]types.SearchResult{}, nil)

	results, err := e.Evaluate(ctx, "col", questions, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.False(t, results[0].Hit[1])
	assert.Equal(t, 0, results[0].RankFirst)
	assert.Empty(t, results[0].Answer)
}

func TestEvaluate_WithJudge_SetsAnswerScore(t *testing.T) {
	r := new(mockRetriever)
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	j := new(mockGenerator)
	e := NewRetrievalEvaluatorWithJudge(r, emb, g, j, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{
			ID:             "q1",
			Question:       "test query",
			ExpectedAnswer: "correct answer",
			Relevance:      []types.RelevanceJudgment{{DocumentPath: "doc1.md", Grade: 3}},
		},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 3).
		Return([]types.SearchResult{{DocumentPath: "doc1.md", Score: 0.9, Content: "content"}}, nil)

	g.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "generated answer"}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 5, CompletionTokens: 10},
		}, nil)

	j.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: `{"score": 0.92, "reasoning": "correct"}`}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 3, CompletionTokens: 1},
		}, nil)

	results, err := e.Evaluate(ctx, "col", questions, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "generated answer", results[0].Answer)
	assert.InDelta(t, 0.92, results[0].AnswerScore, 0.001)
}

func TestEvaluate_SingleQuestion_WithSourceURL(t *testing.T) {
	r := new(mockRetriever)
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{
			ID:             "q1",
			Question:       "test query",
			ExpectedAnswer: "answer",
			Relevance:      []types.RelevanceJudgment{{DocumentPath: "doc1.md", Grade: 3}},
		},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 3).
		Return([]types.SearchResult{
			{
				DocumentPath: "doc1.md",
				Score:        0.9,
				Content:      "relevant content",
				Metadata:     map[string]string{"source_url": "https://example.com/doc1/"},
			},
		}, nil)

	var capturedPrompt string
	g.On("Generate", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			params := args.Get(1).(openai.ChatCompletionNewParams)
			if content, ok := params.Messages[1].GetContent().AsAny().(*string); ok && content != nil {
				capturedPrompt = *content
			}
		}).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "answer"}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 5, CompletionTokens: 10},
		}, nil)

	results, err := e.Evaluate(ctx, "col", questions, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, capturedPrompt, "https://example.com/doc1/")
	assert.NotContains(t, capturedPrompt, "Document: doc1.md")
}

func TestEvaluate_ZeroConcurrency_Defaults(t *testing.T) {
	r := new(mockRetriever)
	emb := new(mockEmbedder)
	g := new(mockGenerator)
	e := NewRetrievalEvaluator(r, emb, g, 3)

	ctx := context.Background()
	questions := []types.EvalQuestion{
		{ID: "q1", Question: "q", Relevance: []types.RelevanceJudgment{{DocumentPath: "d.md", Grade: 1}}},
	}

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	r.On("Retrieve", mock.Anything, "col", mock.Anything, 3).
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
