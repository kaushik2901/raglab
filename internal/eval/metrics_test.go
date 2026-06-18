package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/raglab/internal/types"
)

func TestHitRate(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md"},
			Hit:            map[int]bool{1: true, 3: true},
		},
		{
			QuestionID:     "q2",
			ExpectedPaths:  []string{"doc3.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md"},
			Hit:            map[int]bool{1: false, 3: false},
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
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md", "doc3.md"},
		},
		{
			QuestionID:     "q2",
			ExpectedPaths:  []string{"doc4.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md", "doc3.md"},
		},
	}

	precision := computePrecision(results, []int{3})
	assert.InDelta(t, (1.0/3.0+0.0)/2.0, precision[3], 0.001)
}

func TestRecall(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md"},
		},
		{
			QuestionID:     "q2",
			ExpectedPaths:  []string{"doc3.md", "doc4.md"},
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
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc2.md", "doc3.md"},
		},
	}

	ndcg := computeNDCG(results, []int{3})
	assert.InDelta(t, 1.0, ndcg[3], 0.001)
}

func TestNDCGGraded_Basic(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID: "q1",
			Relevance: []types.RelevanceJudgment{
				{DocumentPath: "doc1.md", Grade: 3},
				{DocumentPath: "doc2.md", Grade: 1},
			},
			ExpectedPaths:  []string{"doc1.md", "doc2.md"},
			RetrievedPaths: []string{"doc1.md", "doc3.md", "doc2.md"},
		},
	}

	ndcg := computeNDCGGraded(results, []int{3})
	actual := ndcg[3]
	assert.Greater(t, actual, 0.9, "doc1 (grade 3) ranked 1st should give high NDCG")
	assert.Less(t, actual, 1.0, "doc2 (grade 1) ranked 3rd should not be perfect")
}

func TestNDCGGraded_AllPerfect(t *testing.T) {
	results := []types.RetrievalResult{
		{
			Relevance:      []types.RelevanceJudgment{{DocumentPath: "doc1.md", Grade: 3}},
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md"},
		},
	}

	ndcg := computeNDCGGraded(results, []int{1})
	assert.InDelta(t, 1.0, ndcg[1], 0.001)
}

func TestNDCGGraded_AllMiss(t *testing.T) {
	results := []types.RetrievalResult{
		{
			Relevance:      []types.RelevanceJudgment{{DocumentPath: "doc1.md", Grade: 3}},
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc2.md"},
		},
	}

	ndcg := computeNDCGGraded(results, []int{1})
	assert.Equal(t, 0.0, ndcg[1])
}

func TestNDCGGraded_MultipleQuestions(t *testing.T) {
	results := []types.RetrievalResult{
		{
			Relevance:      []types.RelevanceJudgment{{DocumentPath: "doc1.md", Grade: 3}},
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md"},
		},
		{
			Relevance:      []types.RelevanceJudgment{{DocumentPath: "doc2.md", Grade: 3}},
			ExpectedPaths:  []string{"doc2.md"},
			RetrievedPaths: []string{"doc3.md"},
		},
	}

	ndcg := computeNDCGGraded(results, []int{1})
	assert.InDelta(t, 0.5, ndcg[1], 0.001)
}

func TestRecall_DuplicatePaths_NotDoubleCounted(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc1.md", "doc2.md"},
		},
	}

	recall := computeRecall(results, []int{3})
	assert.InDelta(t, 1.0, recall[3], 0.001, "same doc at multiple ranks should count once")
}

func TestRecall_DuplicatePaths_NoInflationAbove1(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc1.md", "doc1.md"},
		},
	}

	recall := computeRecall(results, []int{3})
	assert.InDelta(t, 1.0, recall[3], 0.001, "recall must never exceed 1.0 even with duplicates")
}

func TestPrecision_DuplicatePaths_NotDoubleCounted(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc1.md", "doc2.md"},
		},
	}

	precision := computePrecision(results, []int{3})
	assert.InDelta(t, 1.0/3.0, precision[3], 0.001, "duplicate doc should count once in numerator")
}

func TestPrecision_DuplicatePaths_AllRelevant(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc1.md", "doc1.md"},
		},
	}

	precision := computePrecision(results, []int{3})
	assert.InDelta(t, 1.0/3.0, precision[3], 0.001, "only 1 unique relevant doc out of 3 results")
}

func TestNDCG_DuplicatePaths_NotAbove1(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc1.md", "doc2.md"},
		},
	}

	ndcg := computeNDCG(results, []int{3})
	assert.LessOrEqual(t, ndcg[3], 1.0+0.001, "NDCG must not exceed 1.0")
	assert.InDelta(t, 1.0, ndcg[3], 0.001, "doc1 at rank 1, doc2 irrelevant -> ideal retrieval")
}

func TestNDCG_DuplicatePaths_OnlyFirstCounts(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID:     "q1",
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc2.md", "doc1.md", "doc1.md"},
		},
	}

	ndcg := computeNDCG(results, []int{3})
	assert.LessOrEqual(t, ndcg[3], 1.0+0.001, "NDCG must not exceed 1.0")
	assert.Greater(t, ndcg[3], 0.0, "doc1 at rank 2 should still contribute")
}

func TestNDCGGraded_DuplicatePaths_NotAbove1(t *testing.T) {
	results := []types.RetrievalResult{
		{
			QuestionID: "q1",
			Relevance: []types.RelevanceJudgment{
				{DocumentPath: "doc1.md", Grade: 3},
			},
			ExpectedPaths:  []string{"doc1.md"},
			RetrievedPaths: []string{"doc1.md", "doc1.md", "doc2.md"},
		},
	}

	ndcg := computeNDCGGraded(results, []int{3})
	assert.LessOrEqual(t, ndcg[3], 1.0+0.001, "graded NDCG must not exceed 1.0")
	assert.InDelta(t, 1.0, ndcg[3], 0.001, "doc1 (grade 3) at rank 1 is ideal")
}

func TestNDCGGradedForQuestion_DuplicatePaths_NotAbove1(t *testing.T) {
	relevance := []types.RelevanceJudgment{
		{DocumentPath: "doc1.md", Grade: 3},
	}
	retrieved := []string{"doc1.md", "doc1.md", "doc2.md"}

	ndcg := computeNDCGGradedForQuestion(relevance, retrieved, 3)
	assert.LessOrEqual(t, ndcg, 1.0+0.001, "per-question graded NDCG must not exceed 1.0")
	assert.InDelta(t, 1.0, ndcg, 0.001, "doc1 (grade 3) at rank 1 is ideal")
}

func TestGradeForPath(t *testing.T) {
	relevance := []types.RelevanceJudgment{
		{DocumentPath: "doc1.md", Grade: 3},
		{DocumentPath: "doc2.md", Grade: 1},
	}
	assert.InDelta(t, 3.0, gradeForPath(relevance, "doc1.md"), 0.001)
	assert.InDelta(t, 1.0, gradeForPath(relevance, "doc2.md"), 0.001)
	assert.InDelta(t, 0.0, gradeForPath(relevance, "doc3.md"), 0.001)
	assert.InDelta(t, 0.0, gradeForPath(nil, "doc1.md"), 0.001)
}

func TestIdealGradedRelevances(t *testing.T) {
	relevance := []types.RelevanceJudgment{
		{DocumentPath: "a.md", Grade: 1},
		{DocumentPath: "b.md", Grade: 3},
		{DocumentPath: "c.md", Grade: 2},
	}

	ideal := idealGradedRelevances(relevance, 3)
	require.Len(t, ideal, 3)
	assert.InDelta(t, 3.0, ideal[0], 0.001)
	assert.InDelta(t, 2.0, ideal[1], 0.001)
	assert.InDelta(t, 1.0, ideal[2], 0.001)
}

func TestIdealGradedRelevances_Truncated(t *testing.T) {
	relevance := []types.RelevanceJudgment{
		{DocumentPath: "a.md", Grade: 3},
		{DocumentPath: "b.md", Grade: 2},
		{DocumentPath: "c.md", Grade: 1},
	}

	ideal := idealGradedRelevances(relevance, 2)
	require.Len(t, ideal, 2)
	assert.InDelta(t, 3.0, ideal[0], 0.001)
	assert.InDelta(t, 2.0, ideal[1], 0.001)
}

func TestIdealGradedRelevances_Empty(t *testing.T) {
	ideal := idealGradedRelevances(nil, 3)
	assert.Empty(t, ideal)
}

func TestContainsPath(t *testing.T) {
	assert.True(t, containsPath([]string{"doc1.md", "doc2.md"}, "doc1.md"))
	assert.False(t, containsPath([]string{"doc1.md", "doc2.md"}, "doc3.md"))
	assert.False(t, containsPath([]string{}, "doc1.md"))
}

func TestAvgAnswerScore(t *testing.T) {
	results := []types.RetrievalResult{
		{AnswerScore: 0.9},
		{AnswerScore: 0.7},
		{AnswerScore: 0.5},
	}
	assert.InDelta(t, 0.7, computeAvgAnswerScore(results), 0.001)
}

func TestAvgAnswerScore_Empty(t *testing.T) {
	assert.Equal(t, 0.0, computeAvgAnswerScore([]types.RetrievalResult{}))
}

func TestAvgAnswerScore_AllZero(t *testing.T) {
	results := []types.RetrievalResult{
		{AnswerScore: 0},
		{AnswerScore: 0},
	}
	assert.Equal(t, 0.0, computeAvgAnswerScore(results))
}

func TestAvgAnswerScore_Single(t *testing.T) {
	results := []types.RetrievalResult{
		{AnswerScore: 0.85},
	}
	assert.InDelta(t, 0.85, computeAvgAnswerScore(results), 0.001)
}

func TestMin(t *testing.T) {
	assert.Equal(t, 3, min(3, 5))
	assert.Equal(t, 3, min(5, 3))
	assert.Equal(t, 3, min(3, 3))
}
