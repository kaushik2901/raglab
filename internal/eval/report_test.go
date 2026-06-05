package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func TestWriteJSONReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")

	report := &types.EvalReport{
		RunID: "test-run",
		Strategy: types.EvalStrategyConfig{
			Tag:           "eval-test",
			IndexTag:      "idx-test",
			QueryStrategy: "naive-search",
			TopK:          5,
			LLMModel:      "gpt-4o-mini",
		},
		Questions: 2,
		Aggregate: types.AggregateMetrics{
			HitRate: map[int]float64{1: 0.5, 5: 0.5},
			MRR:     0.75,
		},
		PerQuestion: []types.RetrievalResult{
			{QuestionID: "q1", Hit: map[int]bool{1: true}},
			{QuestionID: "q2", Hit: map[int]bool{1: false}},
		},
	}

	err := WriteJSONReport(report, path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var decoded types.EvalReport
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "test-run", decoded.RunID)
	assert.Equal(t, 2, decoded.Questions)
	assert.InDelta(t, 0.5, decoded.Aggregate.HitRate[1], 0.001)
	assert.InDelta(t, 0.75, decoded.Aggregate.MRR, 0.001)
}

func TestWriteJSONReport_CreatesDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "report.json")

	report := &types.EvalReport{RunID: "test"}
	err := WriteJSONReport(report, path)
	require.NoError(t, err)

	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestWriteJSONReport_EmptyReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	report := &types.EvalReport{}
	err := WriteJSONReport(report, path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}
