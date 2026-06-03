package config

import (
	"flag"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_EmptyRepoURL(t *testing.T) {
	cfg := &Config{RepoPath: "/path", OutputPath: "/out", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_EmptyRepoPath(t *testing.T) {
	cfg := &Config{RepoURL: "https://example.com", OutputPath: "/out", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_EmptyOutputPath(t *testing.T) {
	cfg := &Config{RepoURL: "https://example.com", RepoPath: "/path", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		RepoURL:        "https://example.com/repo",
		RepoPath:       "/tmp/repo",
		OutputPath:     "/tmp/output",
		MaxRetries:     3,
		RetryBackoff:   5 * time.Second,
		LogLevel:       "info",
		ChunkStrategy:  "fixed",
		ChunkSize:      512,
		ChunkOverlap:   64,
		EmbeddingModel: "text-embedding-3-small",
		BatchSize:      20,
		LLMBaseURL:     "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_NegativeMaxRetries(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: -1, RetryBackoff: 5 * time.Second,
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_ZeroMaxRetries(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 0, RetryBackoff: 5 * time.Second, ChunkStrategy: "fixed",
		ChunkSize: 512, ChunkOverlap: 64, EmbeddingModel: "text-embedding-3-small",
		BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_ZeroRetryBackoff(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 0,
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_NegativeRetryBackoff(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: -5 * time.Second,
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_AllEmpty(t *testing.T) {
	cfg := &Config{RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_ENV_KEY", "from_env")
	defer os.Unsetenv("TEST_ENV_KEY")

	assert.Equal(t, "from_env", envOrDefault("TEST_ENV_KEY", "default"))
}

func TestEnvOrDefault_Fallback(t *testing.T) {
	assert.Equal(t, "default", envOrDefault("TEST_ENV_KEY_NONEXISTENT", "default"))
}

func TestEnvOrDefault_EmptyVar(t *testing.T) {
	os.Setenv("TEST_ENV_EMPTY", "")
	defer os.Unsetenv("TEST_ENV_EMPTY")

	assert.Equal(t, "default", envOrDefault("TEST_ENV_EMPTY", "default"))
}

func TestIntEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_INT_KEY", "42")
	defer os.Unsetenv("TEST_INT_KEY")

	assert.Equal(t, 42, intEnvOrDefault("TEST_INT_KEY", 1))
}

func TestIntEnvOrDefault_Fallback(t *testing.T) {
	assert.Equal(t, 10, intEnvOrDefault("TEST_INT_NONEXISTENT", 10))
}

func TestIntEnvOrDefault_InvalidValue(t *testing.T) {
	os.Setenv("TEST_INT_INVALID", "not-a-number")
	defer os.Unsetenv("TEST_INT_INVALID")

	assert.Equal(t, 7, intEnvOrDefault("TEST_INT_INVALID", 7))
}

func TestIntEnvOrDefault_EmptyValue(t *testing.T) {
	os.Setenv("TEST_INT_EMPTY", "")
	defer os.Unsetenv("TEST_INT_EMPTY")

	assert.Equal(t, 5, intEnvOrDefault("TEST_INT_EMPTY", 5))
}

func TestDurationEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_DUR_KEY", "10s")
	defer os.Unsetenv("TEST_DUR_KEY")

	assert.Equal(t, 10*time.Second, durationEnvOrDefault("TEST_DUR_KEY", time.Second))
}

func TestDurationEnvOrDefault_Fallback(t *testing.T) {
	assert.Equal(t, 30*time.Second, durationEnvOrDefault("TEST_DUR_NONEXISTENT", 30*time.Second))
}

func TestDurationEnvOrDefault_InvalidValue(t *testing.T) {
	os.Setenv("TEST_DUR_INVALID", "not-a-duration")
	defer os.Unsetenv("TEST_DUR_INVALID")

	assert.Equal(t, 3*time.Second, durationEnvOrDefault("TEST_DUR_INVALID", 3*time.Second))
}

func TestDurationEnvOrDefault_EmptyValue(t *testing.T) {
	os.Setenv("TEST_DUR_EMPTY", "")
	defer os.Unsetenv("TEST_DUR_EMPTY")

	assert.Equal(t, 2*time.Second, durationEnvOrDefault("TEST_DUR_EMPTY", 2*time.Second))
}

func parseTestFlags(args []string) (*Config, error) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := &Config{}

	fs.StringVar(&cfg.RepoURL, "repo-url", "https://gitlab.com/gitlab-com/content-sites/handbook", "")
	fs.StringVar(&cfg.RepoPath, "repo-path", "./handbook", "")
	fs.StringVar(&cfg.OutputPath, "output", "./output", "")
	fs.IntVar(&cfg.MaxRetries, "max-retries", 3, "")
	fs.DurationVar(&cfg.RetryBackoff, "retry-backoff", 5*time.Second, "")
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "")

	fs.StringVar(&cfg.ChunkStrategy, "chunk-strategy", envOrDefault("CHUNK_STRATEGY", "fixed"), "")
	fs.IntVar(&cfg.ChunkSize, "chunk-size", intEnvOrDefault("CHUNK_SIZE", 512), "")
	fs.IntVar(&cfg.ChunkOverlap, "chunk-overlap", intEnvOrDefault("CHUNK_OVERLAP", 64), "")
	fs.StringVar(&cfg.EmbeddingModel, "embedding-model", envOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"), "")
	fs.IntVar(&cfg.BatchSize, "batch-size", intEnvOrDefault("BATCH_SIZE", 20), "")
	fs.StringVar(&cfg.LLMBaseURL, "llm-base-url", envOrDefault("LLM_BASE_URL", "https://api.openai.com/v1"), "")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	cfg.LLMApiKey = os.Getenv("LLM_API_KEY")
	cfg.QdrantURL = envOrDefault("QDRANT_URL", "http://localhost:6333")
	cfg.QdrantAPIKey = os.Getenv("QDRANT_API_KEY")

	return cfg, cfg.Validate()
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := parseTestFlags([]string{})
	require.NoError(t, err)

	assert.Equal(t, "https://gitlab.com/gitlab-com/content-sites/handbook", cfg.RepoURL)
	assert.Equal(t, "./handbook", cfg.RepoPath)
	assert.Equal(t, "./output", cfg.OutputPath)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 5*time.Second, cfg.RetryBackoff)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "fixed", cfg.ChunkStrategy)
	assert.Equal(t, 512, cfg.ChunkSize)
	assert.Equal(t, 64, cfg.ChunkOverlap)
	assert.Equal(t, "text-embedding-3-small", cfg.EmbeddingModel)
	assert.Equal(t, 20, cfg.BatchSize)
	assert.Equal(t, "https://api.openai.com/v1", cfg.LLMBaseURL)
}

func TestValidate_InvalidChunkStrategy(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "unknown",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_ValidChunkStrategies(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	assert.NoError(t, cfg.Validate())
}

func TestValidate_ZeroChunkSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 0, ChunkOverlap: 0,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_NegativeChunkSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: -1, ChunkOverlap: 0,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_NegativeChunkOverlap(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: -1,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_OverlapGTEChunkSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 100, ChunkOverlap: 100,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	assert.Error(t, cfg.Validate())

	cfg2 := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 100, ChunkOverlap: 99,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	assert.NoError(t, cfg2.Validate())
}

func TestValidate_EmptyEmbeddingModel(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_ZeroBatchSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 0, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_NegativeBatchSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: -1, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_EmptyBaseURL(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_DefaultsValid(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkStrategy: "fixed",
		ChunkSize: 512, ChunkOverlap: 64, EmbeddingModel: "text-embedding-3-small",
		BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
	}
	assert.NoError(t, cfg.Validate())
}

func TestChunkStrategyEnv(t *testing.T) {
	os.Setenv("CHUNK_STRATEGY", "fixed")
	defer os.Unsetenv("CHUNK_STRATEGY")
	cfg, err := parseTestFlags([]string{})
	require.NoError(t, err)
	assert.Equal(t, "fixed", cfg.ChunkStrategy)
}

func TestChunkSizeEnv(t *testing.T) {
	os.Setenv("CHUNK_SIZE", "1024")
	defer os.Unsetenv("CHUNK_SIZE")
	cfg, err := parseTestFlags([]string{})
	require.NoError(t, err)
	assert.Equal(t, 1024, cfg.ChunkSize)
}

func TestChunkOverlapEnv(t *testing.T) {
	os.Setenv("CHUNK_OVERLAP", "128")
	defer os.Unsetenv("CHUNK_OVERLAP")
	cfg, err := parseTestFlags([]string{})
	require.NoError(t, err)
	assert.Equal(t, 128, cfg.ChunkOverlap)
}

func TestBatchSizeEnv(t *testing.T) {
	os.Setenv("BATCH_SIZE", "50")
	defer os.Unsetenv("BATCH_SIZE")
	cfg, err := parseTestFlags([]string{})
	require.NoError(t, err)
	assert.Equal(t, 50, cfg.BatchSize)
}

func TestLLMBaseURLEnv(t *testing.T) {
	os.Setenv("LLM_BASE_URL", "http://localhost:1234/v1")
	defer os.Unsetenv("LLM_BASE_URL")
	cfg, err := parseTestFlags([]string{})
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:1234/v1", cfg.LLMBaseURL)
}

func TestQdrantURLEnv(t *testing.T) {
	os.Setenv("QDRANT_URL", "http://qdrant:6333")
	defer os.Unsetenv("QDRANT_URL")
	cfg, err := parseTestFlags([]string{})
	require.NoError(t, err)
	assert.Equal(t, "http://qdrant:6333", cfg.QdrantURL)
}

func TestValidate_ValidWithMinimalFields(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 0, RetryBackoff: 5 * time.Second, ChunkStrategy: "fixed",
		ChunkSize: 512, ChunkOverlap: 0, EmbeddingModel: "text-embedding-3-small",
		BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
	}
	assert.NoError(t, cfg.Validate())
}

func TestLoad_WithFlags(t *testing.T) {
	cfg, err := parseTestFlags([]string{
		"--repo-url", "https://custom.com/repo",
		"--repo-path", "/custom/path",
		"--output", "/custom/output",
		"--max-retries", "5",
		"--retry-backoff", "10s",
		"--log-level", "debug",
		"--chunk-strategy", "fixed",
		"--chunk-size", "256",
		"--chunk-overlap", "32",
		"--embedding-model", "custom-model",
		"--batch-size", "10",
		"--llm-base-url", "http://localhost:8080/v1",
	})
	require.NoError(t, err)

	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 10*time.Second, cfg.RetryBackoff)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "fixed", cfg.ChunkStrategy)
	assert.Equal(t, 256, cfg.ChunkSize)
	assert.Equal(t, 32, cfg.ChunkOverlap)
	assert.Equal(t, "custom-model", cfg.EmbeddingModel)
	assert.Equal(t, 10, cfg.BatchSize)
	assert.Equal(t, "http://localhost:8080/v1", cfg.LLMBaseURL)
}
