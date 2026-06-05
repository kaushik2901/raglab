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

type mockSearcher struct {
	mock.Mock
}

func (m *mockSearcher) Search(ctx context.Context, collection string, queryVector []float32, topK int) ([]types.SearchResult, error) {
	args := m.Called(ctx, collection, queryVector, topK)
	return args.Get(0).([]types.SearchResult), args.Error(1)
}

type pipelineMockGenerator struct {
	mock.Mock
}

func (m *pipelineMockGenerator) Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*openai.ChatCompletion), args.Error(1)
}

func (m *pipelineMockGenerator) ModelName() string {
	args := m.Called()
	return args.String(0)
}

func TestEvaluate_BasicFlow(t *testing.T) {
	emb := new(mockEmbedder)
	searcher := new(mockSearcher)
	gen := new(pipelineMockGenerator)
	judge := new(pipelineMockGenerator)

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
		Return([]types.Embedding{
			{Vector: []float64{0.1, 0.2}},
		}, nil)

	searcher.On("Search", mock.Anything, "col", []float32{0.1, 0.2}, 3).
		Return([]types.SearchResult{
			{DocumentPath: "doc1.md", Score: 0.95, Content: "content"},
			{DocumentPath: "doc2.md", Score: 0.85},
		}, nil)

	gen.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "the answer"}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 10, CompletionTokens: 20},
		}, nil)

	judge.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: `{"score": 0.85, "reasoning": "good"}`}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 3, CompletionTokens: 1},
		}, nil)

	results, err := Evaluate(context.Background(), PipelineArgs{
		Embedder:   emb,
		Searcher:   searcher,
		Generator:  gen,
		JudgeGen:   judge,
		Collection: "col",
		Questions:  questions,
		TopK:       3,
		EmbedBatch: 10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "q1", results[0].QuestionID)
	assert.Equal(t, "test query", results[0].Question)
	assert.True(t, results[0].Hit[1])
	assert.True(t, results[0].Hit[3])
	assert.Equal(t, 1, results[0].RankFirst)
	assert.Equal(t, "the answer", results[0].Answer)
	assert.InDelta(t, 0.85, results[0].AnswerScore, 0.001)
	assert.Equal(t, 10, results[0].PromptTokens)
	assert.Equal(t, 20, results[0].CompletionTokens)
	assert.GreaterOrEqual(t, results[0].LatencyMs, int64(0))
}

func TestPipeline_MultipleQuestions_Ordering(t *testing.T) {
	emb := new(mockEmbedder)
	searcher := new(mockSearcher)
	gen := new(pipelineMockGenerator)

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{
			{Vector: []float64{0.1}},
			{Vector: []float64{0.2}},
		}, nil)

	searcher.On("Search", mock.Anything, "col", []float32{0.1}, 3).
		Return([]types.SearchResult{{DocumentPath: "d1.md", Score: 0.9, Content: "x"}}, nil)
	searcher.On("Search", mock.Anything, "col", []float32{0.2}, 3).
		Return([]types.SearchResult{{DocumentPath: "d2.md", Score: 0.9, Content: "x"}}, nil)

	gen.On("Generate", mock.Anything, mock.Anything).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "ans"}},
			},
			Usage: openai.CompletionUsage{PromptTokens: 1, CompletionTokens: 1},
		}, nil)

	questions := []types.EvalQuestion{
		{ID: "q1", Question: "first", Relevance: []types.RelevanceJudgment{{DocumentPath: "d1.md", Grade: 1}}},
		{ID: "q2", Question: "second", Relevance: []types.RelevanceJudgment{{DocumentPath: "d2.md", Grade: 1}}},
	}

	results, err := Evaluate(context.Background(), PipelineArgs{
		Embedder:   emb,
		Searcher:   searcher,
		Generator:  gen,
		Collection: "col",
		Questions:  questions,
		TopK:       3,
		EmbedBatch: 10,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "q1", results[0].QuestionID)
	assert.Equal(t, "q2", results[1].QuestionID)
}

func TestEvaluate_BatchEmbed_CorrectBatching(t *testing.T) {
	emb := new(mockEmbedder)
	searcher := new(mockSearcher)

	questions := make([]types.EvalQuestion, 4)
	for i := range questions {
		questions[i] = types.EvalQuestion{
			ID:       string(rune('a' + i)),
			Question: "q",
			Relevance: []types.RelevanceJudgment{
				{DocumentPath: "doc.md", Grade: 1},
			},
		}
	}

	// batch size 2 → 2 calls; each call returns 2 embeddings
	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{
			{Vector: []float64{0.1}},
			{Vector: []float64{0.2}},
		}, nil)

	searcher.On("Search", mock.Anything, "col", mock.Anything, 3).
		Return([]types.SearchResult{}, nil)

	results, err := Evaluate(context.Background(), PipelineArgs{
		Embedder:   emb,
		Searcher:   searcher,
		Collection: "col",
		Questions:  questions,
		TopK:       3,
		EmbedBatch: 2,
	})
	require.NoError(t, err)
	require.Len(t, results, 4)
	emb.AssertNumberOfCalls(t, "Embed", 2)
}

func TestPipeline_GeneratorError_NonFatal(t *testing.T) {
	emb := new(mockEmbedder)
	searcher := new(mockSearcher)
	gen := new(pipelineMockGenerator)

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	searcher.On("Search", mock.Anything, "col", []float32{0.1}, 3).
		Return([]types.SearchResult{{DocumentPath: "doc.md", Score: 0.9, Content: "x"}}, nil)

	gen.On("Generate", mock.Anything, mock.Anything).
		Return(nil, assert.AnError)

	results, err := Evaluate(context.Background(), PipelineArgs{
		Embedder:   emb,
		Searcher:   searcher,
		Generator:  gen,
		Collection: "col",
		Questions: []types.EvalQuestion{
			{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentPath: "doc.md", Grade: 1}}},
		},
		TopK:       3,
		EmbedBatch: 10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Hit[1])
	assert.Empty(t, results[0].Answer)
}

func TestEvaluate_EmptyResults(t *testing.T) {
	emb := new(mockEmbedder)
	searcher := new(mockSearcher)

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	searcher.On("Search", mock.Anything, "col", []float32{0.1}, 5).
		Return([]types.SearchResult{}, nil)

	results, err := Evaluate(context.Background(), PipelineArgs{
		Embedder:   emb,
		Searcher:   searcher,
		Collection: "col",
		Questions: []types.EvalQuestion{
			{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentPath: "doc.md", Grade: 1}}},
		},
		TopK:       5,
		EmbedBatch: 10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].Hit[1])
	assert.Equal(t, 0, results[0].RankFirst)
	assert.Empty(t, results[0].Answer)
}

func TestEvaluate_SearchError(t *testing.T) {
	emb := new(mockEmbedder)
	searcher := new(mockSearcher)

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	searcher.On("Search", mock.Anything, "col", []float32{0.1}, 3).
		Return([]types.SearchResult{}, assert.AnError)

	_, err := Evaluate(context.Background(), PipelineArgs{
		Embedder:   emb,
		Searcher:   searcher,
		Collection: "col",
		Questions: []types.EvalQuestion{
			{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentPath: "doc.md", Grade: 1}}},
		},
		TopK:       3,
		EmbedBatch: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search")
}

func TestEvaluate_EmbedError(t *testing.T) {
	emb := new(mockEmbedder)
	searcher := new(mockSearcher)

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{}, assert.AnError)

	_, err := Evaluate(context.Background(), PipelineArgs{
		Embedder:   emb,
		Searcher:   searcher,
		Collection: "col",
		Questions: []types.EvalQuestion{
			{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentPath: "doc.md", Grade: 1}}},
		},
		TopK:       3,
		EmbedBatch: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "phase 1")
}

func TestEvaluate_Miss(t *testing.T) {
	emb := new(mockEmbedder)
	searcher := new(mockSearcher)

	emb.On("Embed", mock.Anything, mock.Anything).
		Return([]types.Embedding{{Vector: []float64{0.1}}}, nil)

	searcher.On("Search", mock.Anything, "col", []float32{0.1}, 3).
		Return([]types.SearchResult{
			{DocumentPath: "doc1.md", Score: 0.9},
			{DocumentPath: "doc2.md", Score: 0.8},
		}, nil)

	results, err := Evaluate(context.Background(), PipelineArgs{
		Embedder:   emb,
		Searcher:   searcher,
		Collection: "col",
		Questions: []types.EvalQuestion{
			{ID: "q1", Question: "query", Relevance: []types.RelevanceJudgment{{DocumentPath: "docX.md", Grade: 3}}},
		},
		TopK:       3,
		EmbedBatch: 10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].Hit[1])
	assert.False(t, results[0].Hit[3])
	assert.Equal(t, 0, results[0].RankFirst)
}

func TestToFloat32(t *testing.T) {
	in := []float64{0.1, 0.2, 0.3}
	out := toFloat32(in)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, out)
}

func TestToFloat32_Empty(t *testing.T) {
	assert.Empty(t, toFloat32(nil))
	assert.Empty(t, toFloat32([]float64{}))
}

func TestBuildContextText(t *testing.T) {
	results := []types.SearchResult{
		{DocumentPath: "doc1.md", Content: "content one"},
		{DocumentPath: "doc2.md", Content: "content two"},
	}
	text := buildContextText(results)
	assert.Contains(t, text, "Document: doc1.md")
	assert.Contains(t, text, "content one")
	assert.Contains(t, text, "Document: doc2.md")
	assert.Contains(t, text, "content two")
	assert.Contains(t, text, "---")
}

func TestBuildContextText_Empty(t *testing.T) {
	assert.Empty(t, buildContextText(nil))
	assert.Empty(t, buildContextText([]types.SearchResult{}))
}

func TestBatchEmbedQueries_Empty(t *testing.T) {
	emb := new(mockEmbedder)
	vectors, err := batchEmbedQueries(context.Background(), emb, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, vectors)
}

func TestFillRetrievalResult_Hit(t *testing.T) {
	r := &types.RetrievalResult{}
	q := types.EvalQuestion{
		ID:       "q1",
		Question: "test",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc1.md", Grade: 3},
		},
	}
	searchResults := []types.SearchResult{
		{DocumentPath: "doc1.md", Score: 0.95},
		{DocumentPath: "doc2.md", Score: 0.85},
	}

	fillRetrievalResult(r, q, searchResults, 3)
	assert.Equal(t, "q1", r.QuestionID)
	assert.Equal(t, "test", r.Question)
	assert.Equal(t, []string{"doc1.md"}, r.ExpectedPaths)
	assert.Equal(t, []string{"doc1.md", "doc2.md"}, r.RetrievedPaths)
	assert.True(t, r.Hit[1])
	assert.True(t, r.Hit[2])
	assert.True(t, r.Hit[3])
	assert.Equal(t, 1, r.RankFirst)
}

func TestFillRetrievalResult_Miss(t *testing.T) {
	r := &types.RetrievalResult{}
	q := types.EvalQuestion{
		ID:       "q1",
		Question: "test",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc3.md", Grade: 3},
		},
	}
	searchResults := []types.SearchResult{
		{DocumentPath: "doc1.md", Score: 0.95},
		{DocumentPath: "doc2.md", Score: 0.85},
	}

	fillRetrievalResult(r, q, searchResults, 3)
	assert.False(t, r.Hit[1])
	assert.False(t, r.Hit[3])
	assert.Equal(t, 0, r.RankFirst)
}

func TestFillRetrievalResult_EmptySearch(t *testing.T) {
	r := &types.RetrievalResult{}
	q := types.EvalQuestion{
		ID:       "q1",
		Question: "test",
		Relevance: []types.RelevanceJudgment{
			{DocumentPath: "doc1.md", Grade: 3},
		},
	}

	fillRetrievalResult(r, q, nil, 3)
	assert.Empty(t, r.RetrievedPaths)
	assert.False(t, r.Hit[1])
	assert.Equal(t, 0, r.RankFirst)
}
