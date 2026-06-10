package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreprocessRequest_Validate_Valid(t *testing.T) {
	t.Parallel()
	err := PreprocessRequest{RepoURL: "https://example.com/repo.git", Tag: "my-tag"}.Validate()
	assert.NoError(t, err)
}

func TestPreprocessRequest_Validate_MissingRepoURL(t *testing.T) {
	t.Parallel()
	err := PreprocessRequest{Tag: "t"}.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repo_url")
}

func TestPreprocessRequest_Validate_MissingTag(t *testing.T) {
	t.Parallel()
	err := PreprocessRequest{RepoURL: "https://example.com/r"}.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag")
}

func TestPreprocessRequest_Validate_AllMissing(t *testing.T) {
	t.Parallel()
	err := PreprocessRequest{}.Validate()
	assert.Error(t, err)
}

func TestIndexRequest_Validate_Valid(t *testing.T) {
	t.Parallel()
	err := IndexRequest{
		InputTag:          "pre-tag",
		Tag:               "idx-tag",
		ParserStrategy:    "markdown",
		ChunkStrategy:     "fixed",
		ChunkSize:         512,
		ChunkOverlap:      64,
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
		BatchSize:         20,
		IndexConcurrency:  5,
		DocTimeout:        "30m",
	}.Validate()
	assert.NoError(t, err)
}

func TestIndexRequest_Validate_MissingInputTag(t *testing.T) {
	t.Parallel()
	err := IndexRequest{
		Tag: "t", ParserStrategy: "m", ChunkStrategy: "f",
		ChunkSize: 1, EmbeddingProvider: "o", EmbeddingModel: "m",
		BatchSize: 1, IndexConcurrency: 1, DocTimeout: "30m",
	}.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input_tag")
}

func TestIndexRequest_Validate_MissingTag(t *testing.T) {
	t.Parallel()
	err := IndexRequest{
		InputTag: "pre", ParserStrategy: "m", ChunkStrategy: "f",
		ChunkSize: 1, EmbeddingProvider: "o", EmbeddingModel: "m",
		BatchSize: 1, IndexConcurrency: 1, DocTimeout: "30m",
	}.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag")
}

func TestIndexRequest_Validate_InvalidChunkSize(t *testing.T) {
	t.Parallel()
	err := IndexRequest{
		InputTag: "pre", Tag: "t", ParserStrategy: "m",
		ChunkStrategy: "f", ChunkSize: 0, ChunkOverlap: 0,
		EmbeddingProvider: "o", EmbeddingModel: "m",
		BatchSize: 1, IndexConcurrency: 1, DocTimeout: "30m",
	}.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chunk_size")
}

func TestIndexRequest_Validate_InvalidBatchSize(t *testing.T) {
	t.Parallel()
	err := IndexRequest{
		InputTag: "pre", Tag: "t", ParserStrategy: "m",
		ChunkStrategy: "f", ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingProvider: "o", EmbeddingModel: "m",
		BatchSize: 0, IndexConcurrency: 1, DocTimeout: "30m",
	}.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "batch_size")
}

func TestEvalRequest_Validate_Valid(t *testing.T) {
	t.Parallel()
	err := EvalRequest{
		IndexTag:          "idx-tag",
		Tag:               "eval-tag",
		QueryStrategy:     "naive-search",
		DatasetPath:       "/data/dataset.jsonl",
		Ks:                []int{1, 3, 5},
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
		JudgeProvider:     "openai",
		JudgeModel:        "gpt-4o-mini",
		BatchSize:         20,
		Workers:           5,
	}.Validate()
	assert.NoError(t, err)
}

func TestEvalRequest_Validate_MissingFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		req   EvalRequest
		field string
	}{
		{"missing index_tag", EvalRequest{Tag: "t", QueryStrategy: "q", DatasetPath: "/d", Ks: []int{1}, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m", JudgeProvider: "o", JudgeModel: "m", BatchSize: 1, Workers: 1}, "index_tag"},
		{"missing tag", EvalRequest{IndexTag: "i", QueryStrategy: "q", DatasetPath: "/d", Ks: []int{1}, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m", JudgeProvider: "o", JudgeModel: "m", BatchSize: 1, Workers: 1}, "tag"},
		{"missing query_strategy", EvalRequest{IndexTag: "i", Tag: "t", DatasetPath: "/d", Ks: []int{1}, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m", JudgeProvider: "o", JudgeModel: "m", BatchSize: 1, Workers: 1}, "query_strategy"},
		{"missing dataset_path", EvalRequest{IndexTag: "i", Tag: "t", QueryStrategy: "q", Ks: []int{1}, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m", JudgeProvider: "o", JudgeModel: "m", BatchSize: 1, Workers: 1}, "dataset_path"},
		{"missing llm_provider", EvalRequest{IndexTag: "i", Tag: "t", QueryStrategy: "q", DatasetPath: "/d", Ks: []int{1}, LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m", JudgeProvider: "o", JudgeModel: "m", BatchSize: 1, Workers: 1}, "llm_provider"},
		{"missing workers", EvalRequest{IndexTag: "i", Tag: "t", QueryStrategy: "q", DatasetPath: "/d", Ks: []int{1}, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m", JudgeProvider: "o", JudgeModel: "m", BatchSize: 1, Workers: 0}, "workers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.field)
		})
	}
}

func TestChatRequest_Validate_Valid(t *testing.T) {
	t.Parallel()
	err := ChatRequest{
		Tag:               "my-collection",
		Query:             "test query",
		TopK:              5,
		Temperature:       0.3,
		MaxTokens:         1024,
		LLMProvider:       "openai",
		LLMModel:          "gpt-4o-mini",
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
	}.Validate()
	assert.NoError(t, err)
}

func TestChatRequest_Validate_MissingFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		req   ChatRequest
		field string
	}{
		{"missing tag", ChatRequest{Query: "q", TopK: 1, Temperature: 0.5, MaxTokens: 100, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m"}, "tag"},
		{"missing query", ChatRequest{Tag: "t", TopK: 1, Temperature: 0.5, MaxTokens: 100, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m"}, "query"},
		{"missing top_k", ChatRequest{Tag: "t", Query: "q", Temperature: 0.5, MaxTokens: 100, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m"}, "top_k"},
		{"missing temperature", ChatRequest{Tag: "t", Query: "q", TopK: 1, MaxTokens: 100, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m"}, "temperature"},
		{"missing max_tokens", ChatRequest{Tag: "t", Query: "q", TopK: 1, Temperature: 0.5, LLMProvider: "o", LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m"}, "max_tokens"},
		{"missing llm_provider", ChatRequest{Tag: "t", Query: "q", TopK: 1, Temperature: 0.5, MaxTokens: 100, LLMModel: "m", EmbeddingProvider: "o", EmbeddingModel: "m"}, "llm_provider"},
		{"missing embedding_provider", ChatRequest{Tag: "t", Query: "q", TopK: 1, Temperature: 0.5, MaxTokens: 100, LLMProvider: "o", LLMModel: "m", EmbeddingModel: "m"}, "embedding_provider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.field)
		})
	}
}
