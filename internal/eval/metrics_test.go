package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestHitRate(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID: "q1",
			ExpectedPaths: []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md"},
			Hit: map[int]bool{1: true, 3: true},
		},
		{
			QuestionID: "q2",
			ExpectedPaths: []string{"doc3.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md"},
			Hit: map[int]bool{1: false, 3: false},
		},
	}

	hitRate := computeHitRate(results, []int{1, 3})
	assert.Equal(t, 0.5, hitRate[1])
	assert.Equal(t, 0.5, hitRate[3])
}

func TestMRR(t *testing.T) {
	results := []types.RetrievalResult{
		{QuestionID: "q1", RankFirst: 1},
		{QuestionID: "q2", RankFirst: 3},
		{QuestionID: "q3", RankFirst: 0},
	}

	mrr := computeMRR(results)
	assert.InDelta(t, (1.0+1.0/3.0+0.0)/3.0, mrr, 0.001)
}

func TestMRR_AllRankFirst(t *testing.T) {
	results := []types.RetrievalResult{
		{QuestionID: "q1", RankFirst: 1},
		{QuestionID: "q2", RankFirst: 1},
	}

	mrr := computeMRR(results)
	assert.Equal(t, 1.0, mrr)
}

func TestMRR_NoResults(t *testing.T) {
	assert.Equal(t, 0.0, computeMRR([]types.RetrievalResult{}))
}

func TestPrecision(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID: "q1",
			ExpectedPaths: []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md", "doc3.md"},
		},
		{
			QuestionID: "q2",
			ExpectedPaths: []string{"doc4.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md", "doc3.md"},
		},
	}

	precision := computePrecision(results, []int{3})
	assert.InDelta(t, (1.0/3.0+0.0)/2.0, precision[3], 0.001)
}

func TestRecall(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID: "q1",
			ExpectedPaths: []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md"},
		},
		{
			QuestionID: "q2",
			ExpectedPaths: []string{"doc3.md", "doc4.md"},
			RetrievedPaths: []string{"doc1.md", "doc3.md"},
		},
	}

	recall := computeRecall(results, []int{2})
	assert.InDelta(t, 0.75, recall[2], 0.001)

	recall2 := computeRecall(results, []int{5})
	assert.Equal(t, 0.75, recall2[5])
}

func TestNDCG(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID: "q1",
			ExpectedPaths: []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md", "doc3.md"},
		},
	}

	ndcg := computeNDCG(results, []int{3})
	assert.InDelta(t, 1.0, ndcg[3], 0.001)
}

func TestContainsPath(t *testing.T) {
	assert.True(t, containsPath([]string{"doc1.md", "doc2.md"}, "doc1.md"))
	assert.False(t, containsPath([]string{"doc1.md", "doc2.md"}, "doc3.md"))
	assert.False(t, containsPath([]string{}, "doc1.md"))
}

func TestMin(t *testing.T) {
	assert.Equal(t, 3, min(3, 5))
	assert.Equal(t, 3, min(5, 3))
	assert.Equal(t, 3, min(3, 3))
}
