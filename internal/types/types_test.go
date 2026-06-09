package types

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentCreation(t *testing.T) {
	doc := Document{
		Path:    "content/docs/foo.md",
		Content: "# Hello\nWorld",
		Size:    18,
	}
	assert.Equal(t, "content/docs/foo.md", doc.Path)
	assert.Equal(t, "# Hello\nWorld", doc.Content)
	assert.Equal(t, int64(18), doc.Size)
}

func TestDocumentZeroValue(t *testing.T) {
	var doc Document
	assert.Equal(t, "", doc.Path)
	assert.Equal(t, "", doc.Content)
	assert.Equal(t, int64(0), doc.Size)
}

func TestStageResultCreation(t *testing.T) {
	result := StageResult{
		Name:   "clone",
		Output: map[string]any{"repo_path": "/tmp/repo"},
		Err:    nil,
	}
	assert.Equal(t, StageID("clone"), result.Name)
	assert.NoError(t, result.Err)
	assert.Equal(t, "/tmp/repo", result.Output["repo_path"])
}

func TestStageResultWithError(t *testing.T) {
	err := errors.New("network error")
	result := StageResult{
		Name: "clone",
		Err:  err,
	}
	assert.Error(t, result.Err)
	assert.Equal(t, "network error", result.Err.Error())
}

func TestStageResultNilOutput(t *testing.T) {
	result := StageResult{Name: "verify"}
	assert.Nil(t, result.Output)
}

func TestSearchResultCreation(t *testing.T) {
	sr := SearchResult{
		ChunkID:      "chunk-001",
		DocumentPath: "docs/page.md",
		Content:      "some text",
		Score:        0.95,
		TokenCount:   42,
		ChunkIndex:   1,
	}
	assert.Equal(t, "chunk-001", sr.ChunkID)
	assert.Equal(t, "docs/page.md", sr.DocumentPath)
	assert.Equal(t, "some text", sr.Content)
	assert.Equal(t, float32(0.95), sr.Score)
	assert.Equal(t, 42, sr.TokenCount)
	assert.Equal(t, 1, sr.ChunkIndex)
}

func TestSearchResultZeroValue(t *testing.T) {
	var sr SearchResult
	assert.Empty(t, sr.ChunkID)
	assert.Empty(t, sr.DocumentPath)
	assert.Empty(t, sr.Content)
	assert.Equal(t, float32(0), sr.Score)
	assert.Equal(t, 0, sr.TokenCount)
	assert.Equal(t, 0, sr.ChunkIndex)
}

func TestEvalQuestionCreation(t *testing.T) {
	q := EvalQuestion{
		ID:             "q-001",
		Category:       "onboarding",
		Difficulty:     "easy",
		Question:       "How do I set up SSH?",
		ExpectedAnswer: "Run ssh-keygen",
		Relevance: []RelevanceJudgment{
			{DocumentPath: "docs/ssh.md", Grade: 3},
		},
	}
	assert.Equal(t, "q-001", q.ID)
	assert.Equal(t, "onboarding", q.Category)
	assert.Equal(t, "easy", q.Difficulty)
	assert.Equal(t, "Run ssh-keygen", q.ExpectedAnswer)
	require.Len(t, q.Relevance, 1)
	assert.Equal(t, "docs/ssh.md", q.Relevance[0].DocumentPath)
	assert.Equal(t, 3, q.Relevance[0].Grade)
}

func TestEvalDatasetCreation(t *testing.T) {
	ds := EvalDataset{
		Meta: EvalDatasetMeta{
			Created:     "2024-01-01",
			Version:     1,
			Description: "Test dataset",
		},
		Questions: []EvalQuestion{
			{ID: "q1", Question: "test", Relevance: []RelevanceJudgment{{DocumentPath: "doc.md", Grade: 1}}},
		},
	}
	assert.Equal(t, "2024-01-01", ds.Meta.Created)
	assert.Equal(t, 1, ds.Meta.Version)
	assert.Len(t, ds.Questions, 1)
}

func TestRetrievalResultCreation(t *testing.T) {
	r := RetrievalResult{
		QuestionID:       "q1",
		Question:         "test?",
		ExpectedAnswer:   "expected",
		Relevance:        []RelevanceJudgment{{DocumentPath: "doc.md", Grade: 3}},
		ExpectedPaths:    []string{"doc.md"},
		RetrievedPaths:   []string{"doc.md", "doc2.md"},
		Scores:           []float64{0.95, 0.85},
		Hit:              map[int]bool{1: true, 3: true},
		RankFirst:        1,
		NDCGGraded:       0.95,
		Answer:           "answer text",
		AnswerScore:      0.88,
		PromptTokens:     10,
		CompletionTokens: 20,
		LatencyMs:        150,
	}
	assert.Equal(t, "q1", r.QuestionID)
	assert.Equal(t, []string{"doc.md"}, r.ExpectedPaths)
	assert.Equal(t, "answer text", r.Answer)
	assert.Equal(t, "expected", r.ExpectedAnswer)
	assert.Equal(t, int64(150), r.LatencyMs)
	assert.InDelta(t, 0.95, r.NDCGGraded, 0.001)
	assert.InDelta(t, 0.88, r.AnswerScore, 0.001)
}

func TestAggregateMetricsDefaults(t *testing.T) {
	m := AggregateMetrics{}
	assert.Nil(t, m.HitRate)
	assert.Equal(t, 0.0, m.MRR)
	assert.Nil(t, m.NDCG)
	assert.Nil(t, m.NDCGGraded)
	assert.Nil(t, m.Precision)
	assert.Nil(t, m.Recall)
	assert.Equal(t, 0.0, m.AvgAnswerScore)
}
