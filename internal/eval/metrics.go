package eval

import (
	"math"
	"sort"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func computeHitRate(results []types.RetrievalResult, ks []int) map[int]float64 {
	out := make(map[int]float64, len(ks))
	for _, k := range ks {
		hits := 0
		for _, r := range results {
			for i := 0; i < min(k, len(r.RetrievedPaths)); i++ {
				if containsPath(r.ExpectedPaths, r.RetrievedPaths[i]) {
					hits++
					break
				}
			}
		}
		out[k] = float64(hits) / float64(len(results))
	}
	return out
}

func computeMRR(results []types.RetrievalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	sum := 0.0
	for _, r := range results {
		if r.RankFirst > 0 {
			sum += 1.0 / float64(r.RankFirst)
		}
	}
	return sum / float64(len(results))
}

func computeNDCG(results []types.RetrievalResult, ks []int) map[int]float64 {
	out := make(map[int]float64, len(ks))
	for _, k := range ks {
		sum := 0.0
		for _, r := range results {
			relevances := make([]float64, min(k, len(r.RetrievedPaths)))
			for i := 0; i < len(relevances); i++ {
				if containsPath(r.ExpectedPaths, r.RetrievedPaths[i]) {
					relevances[i] = 1.0
				}
			}
			dcgVal := computeDCG(relevances, k)
			ideal := make([]float64, len(relevances))
			for i := 0; i < min(len(relevances), len(r.ExpectedPaths)); i++ {
				ideal[i] = 1.0
			}
			sort.Sort(sort.Reverse(sort.Float64Slice(ideal)))
			idcg := computeDCG(ideal, k)
			if idcg > 0 {
				sum += dcgVal / idcg
			}
		}
		out[k] = sum / float64(len(results))
	}
	return out
}

func computeDCG(relevances []float64, k int) float64 {
	val := 0.0
	for i := 0; i < min(k, len(relevances)); i++ {
		num := math.Pow(2, relevances[i]) - 1
		den := math.Log2(float64(i + 2))
		val += num / den
	}
	return val
}

func computePrecision(results []types.RetrievalResult, ks []int) map[int]float64 {
	out := make(map[int]float64, len(ks))
	for _, k := range ks {
		sum := 0.0
		for _, r := range results {
			relevant := 0
			for i := 0; i < min(k, len(r.RetrievedPaths)); i++ {
				if containsPath(r.ExpectedPaths, r.RetrievedPaths[i]) {
					relevant++
				}
			}
			sum += float64(relevant) / float64(k)
		}
		out[k] = sum / float64(len(results))
	}
	return out
}

func computeRecall(results []types.RetrievalResult, ks []int) map[int]float64 {
	out := make(map[int]float64, len(ks))
	for _, k := range ks {
		sum := 0.0
		for _, r := range results {
			if len(r.ExpectedPaths) == 0 {
				continue
			}
			relevant := 0
			for i := 0; i < min(k, len(r.RetrievedPaths)); i++ {
				if containsPath(r.ExpectedPaths, r.RetrievedPaths[i]) {
					relevant++
				}
			}
			sum += float64(relevant) / float64(len(r.ExpectedPaths))
		}
		out[k] = sum / float64(len(results))
	}
	return out
}

func computeNDCGGraded(results []types.RetrievalResult, ks []int) map[int]float64 {
	out := make(map[int]float64, len(ks))
	for _, k := range ks {
		sum := 0.0
		for _, r := range results {
			relevances := make([]float64, min(k, len(r.RetrievedPaths)))
			for i, path := range r.RetrievedPaths[:min(k, len(r.RetrievedPaths))] {
				relevances[i] = gradeForPath(r.Relevance, path)
			}
			dcgVal := computeDCG(relevances, k)
			ideal := idealGradedRelevances(r.Relevance, k)
			idcg := computeDCG(ideal, k)
			if idcg > 0 {
				sum += dcgVal / idcg
			}
		}
		out[k] = sum / float64(len(results))
	}
	return out
}

func gradeForPath(relevance []types.RelevanceJudgment, path string) float64 {
	for _, j := range relevance {
		if j.DocumentID == path {
			return float64(j.Grade)
		}
	}
	return 0
}

func idealGradedRelevances(relevance []types.RelevanceJudgment, k int) []float64 {
	grades := make([]float64, len(relevance))
	for i, j := range relevance {
		grades[i] = float64(j.Grade)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(grades)))
	if len(grades) > k {
		grades = grades[:k]
	}
	return grades
}

func ComputeAggregateMetrics(results []types.RetrievalResult, ks []int) types.AggregateMetrics {
	return types.AggregateMetrics{
		HitRate:    computeHitRate(results, ks),
		MRR:        computeMRR(results),
		NDCG:       computeNDCG(results, ks),
		NDCGGraded: computeNDCGGraded(results, ks),
		Precision:  computePrecision(results, ks),
		Recall:     computeRecall(results, ks),
	}
}

func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if matchPath(p, target) {
			return true
		}
	}
	return false
}

func matchPath(expected, actual string) bool {
	return expected == actual
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
