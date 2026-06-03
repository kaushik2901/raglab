package config

import (
	"flag"
	"os"
	"testing"
	"time"
)

func TestValidate_EmptyRepoURL(t *testing.T) {
	cfg := &Config{RepoPath: "/path", OutputPath: "/out", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty RepoURL")
	}
}

func TestValidate_EmptyRepoPath(t *testing.T) {
	cfg := &Config{RepoURL: "https://example.com", OutputPath: "/out", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty RepoPath")
	}
}

func TestValidate_EmptyOutputPath(t *testing.T) {
	cfg := &Config{RepoURL: "https://example.com", RepoPath: "/path", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty OutputPath")
	}
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
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_NegativeMaxRetries(t *testing.T) {
	cfg := &Config{
		RepoURL:      "https://example.com",
		RepoPath:     "/path",
		OutputPath:   "/out",
		MaxRetries:   -1,
		RetryBackoff: 5 * time.Second,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative MaxRetries")
	}
}

func TestValidate_ZeroMaxRetries(t *testing.T) {
	cfg := &Config{
		RepoURL:        "https://example.com",
		RepoPath:       "/path",
		OutputPath:     "/out",
		MaxRetries:     0,
		RetryBackoff:   5 * time.Second,
		ChunkStrategy:  "fixed",
		ChunkSize:      512,
		ChunkOverlap:   64,
		EmbeddingModel: "text-embedding-3-small",
		BatchSize:      20,
		LLMBaseURL:     "https://api.openai.com/v1",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil for zero MaxRetries, got: %v", err)
	}
}

func TestValidate_ZeroRetryBackoff(t *testing.T) {
	cfg := &Config{
		RepoURL:      "https://example.com",
		RepoPath:     "/path",
		OutputPath:   "/out",
		MaxRetries:   3,
		RetryBackoff: 0,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for zero RetryBackoff")
	}
}

func TestValidate_NegativeRetryBackoff(t *testing.T) {
	cfg := &Config{
		RepoURL:      "https://example.com",
		RepoPath:     "/path",
		OutputPath:   "/out",
		MaxRetries:   3,
		RetryBackoff: -5 * time.Second,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative RetryBackoff")
	}
}

func TestValidate_AllEmpty(t *testing.T) {
	cfg := &Config{RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for all empty required fields")
	}
}

func TestEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_ENV_KEY", "from_env")
	defer os.Unsetenv("TEST_ENV_KEY")

	if got := envOrDefault("TEST_ENV_KEY", "default"); got != "from_env" {
		t.Errorf("envOrDefault = %q, want %q", got, "from_env")
	}
}

func TestEnvOrDefault_Fallback(t *testing.T) {
	if got := envOrDefault("TEST_ENV_KEY_NONEXISTENT", "default"); got != "default" {
		t.Errorf("envOrDefault = %q, want %q", got, "default")
	}
}

func TestEnvOrDefault_EmptyVar(t *testing.T) {
	os.Setenv("TEST_ENV_EMPTY", "")
	defer os.Unsetenv("TEST_ENV_EMPTY")

	if got := envOrDefault("TEST_ENV_EMPTY", "default"); got != "default" {
		t.Errorf("envOrDefault = %q, want %q", got, "default")
	}
}

func TestIntEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_INT_KEY", "42")
	defer os.Unsetenv("TEST_INT_KEY")

	if got := intEnvOrDefault("TEST_INT_KEY", 1); got != 42 {
		t.Errorf("intEnvOrDefault = %d, want %d", got, 42)
	}
}

func TestIntEnvOrDefault_Fallback(t *testing.T) {
	if got := intEnvOrDefault("TEST_INT_NONEXISTENT", 10); got != 10 {
		t.Errorf("intEnvOrDefault = %d, want %d", got, 10)
	}
}

func TestIntEnvOrDefault_InvalidValue(t *testing.T) {
	os.Setenv("TEST_INT_INVALID", "not-a-number")
	defer os.Unsetenv("TEST_INT_INVALID")

	if got := intEnvOrDefault("TEST_INT_INVALID", 7); got != 7 {
		t.Errorf("intEnvOrDefault = %d, want %d", got, 7)
	}
}

func TestIntEnvOrDefault_EmptyValue(t *testing.T) {
	os.Setenv("TEST_INT_EMPTY", "")
	defer os.Unsetenv("TEST_INT_EMPTY")

	if got := intEnvOrDefault("TEST_INT_EMPTY", 5); got != 5 {
		t.Errorf("intEnvOrDefault = %d, want %d", got, 5)
	}
}

func TestDurationEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_DUR_KEY", "10s")
	defer os.Unsetenv("TEST_DUR_KEY")

	if got := durationEnvOrDefault("TEST_DUR_KEY", time.Second); got != 10*time.Second {
		t.Errorf("durationEnvOrDefault = %v, want %v", got, 10*time.Second)
	}
}

func TestDurationEnvOrDefault_Fallback(t *testing.T) {
	if got := durationEnvOrDefault("TEST_DUR_NONEXISTENT", 30*time.Second); got != 30*time.Second {
		t.Errorf("durationEnvOrDefault = %v, want %v", got, 30*time.Second)
	}
}

func TestDurationEnvOrDefault_InvalidValue(t *testing.T) {
	os.Setenv("TEST_DUR_INVALID", "not-a-duration")
	defer os.Unsetenv("TEST_DUR_INVALID")

	if got := durationEnvOrDefault("TEST_DUR_INVALID", 3*time.Second); got != 3*time.Second {
		t.Errorf("durationEnvOrDefault = %v, want %v", got, 3*time.Second)
	}
}

func TestDurationEnvOrDefault_EmptyValue(t *testing.T) {
	os.Setenv("TEST_DUR_EMPTY", "")
	defer os.Unsetenv("TEST_DUR_EMPTY")

	if got := durationEnvOrDefault("TEST_DUR_EMPTY", 2*time.Second); got != 2*time.Second {
		t.Errorf("durationEnvOrDefault = %v, want %v", got, 2*time.Second)
	}
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
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.RepoURL != "https://gitlab.com/gitlab-com/content-sites/handbook" {
		t.Errorf("RepoURL = %q, want %q", cfg.RepoURL, "https://gitlab.com/gitlab-com/content-sites/handbook")
	}
	if cfg.RepoPath != "./handbook" {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, "./handbook")
	}
	if cfg.OutputPath != "./output" {
		t.Errorf("OutputPath = %q, want %q", cfg.OutputPath, "./output")
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, 3)
	}
	if cfg.RetryBackoff != 5*time.Second {
		t.Errorf("RetryBackoff = %v, want %v", cfg.RetryBackoff, 5*time.Second)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.ChunkStrategy != "fixed" {
		t.Errorf("ChunkStrategy = %q, want %q", cfg.ChunkStrategy, "fixed")
	}
	if cfg.ChunkSize != 512 {
		t.Errorf("ChunkSize = %d, want %d", cfg.ChunkSize, 512)
	}
	if cfg.ChunkOverlap != 64 {
		t.Errorf("ChunkOverlap = %d, want %d", cfg.ChunkOverlap, 64)
	}
	if cfg.EmbeddingModel != "text-embedding-3-small" {
		t.Errorf("EmbeddingModel = %q, want %q", cfg.EmbeddingModel, "text-embedding-3-small")
	}
	if cfg.BatchSize != 20 {
		t.Errorf("BatchSize = %d, want %d", cfg.BatchSize, 20)
	}
	if cfg.LLMBaseURL != "https://api.openai.com/v1" {
		t.Errorf("LLMBaseURL = %q, want %q", cfg.LLMBaseURL, "https://api.openai.com/v1")
	}
}

func TestValidate_InvalidChunkStrategy(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "unknown",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid chunk strategy")
	}
}

func TestValidate_ValidChunkStrategies(t *testing.T) {
	for _, strategy := range []string{"fixed", "semantic", "recursive"} {
		cfg := &Config{
			RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
			MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
			EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
			ChunkStrategy: strategy,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error for strategy %q: %v", strategy, err)
		}
	}
}

func TestValidate_ZeroChunkSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 0, ChunkOverlap: 0,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for zero chunk-size")
	}
}

func TestValidate_NegativeChunkSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: -1, ChunkOverlap: 0,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative chunk-size")
	}
}

func TestValidate_NegativeChunkOverlap(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: -1,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative chunk-overlap")
	}
}

func TestValidate_OverlapGTEChunkSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 100, ChunkOverlap: 100,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for overlap >= size")
	}

	cfg2 := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 100, ChunkOverlap: 99,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	if err := cfg2.Validate(); err != nil {
		t.Errorf("unexpected error for overlap = size-1: %v", err)
	}
}

func TestValidate_EmptyEmbeddingModel(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "", BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty embedding-model")
	}
}

func TestValidate_ZeroBatchSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 0, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for zero batch-size")
	}
}

func TestValidate_NegativeBatchSize(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: -1, LLMBaseURL: "https://api.openai.com/v1",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative batch-size")
	}
}

func TestValidate_EmptyBaseURL(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkSize: 512, ChunkOverlap: 64,
		EmbeddingModel: "text-embedding-3-small", BatchSize: 20, LLMBaseURL: "",
		ChunkStrategy: "fixed",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty llm-base-url")
	}
}

func TestValidate_DefaultsValid(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 3, RetryBackoff: 5 * time.Second, ChunkStrategy: "fixed",
		ChunkSize: 512, ChunkOverlap: 64, EmbeddingModel: "text-embedding-3-small",
		BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChunkStrategyEnv(t *testing.T) {
	os.Setenv("CHUNK_STRATEGY", "semantic")
	defer os.Unsetenv("CHUNK_STRATEGY")
	cfg, err := parseTestFlags([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChunkStrategy != "semantic" {
		t.Errorf("ChunkStrategy = %q, want %q", cfg.ChunkStrategy, "semantic")
	}
}

func TestChunkSizeEnv(t *testing.T) {
	os.Setenv("CHUNK_SIZE", "1024")
	defer os.Unsetenv("CHUNK_SIZE")
	cfg, err := parseTestFlags([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChunkSize != 1024 {
		t.Errorf("ChunkSize = %d, want %d", cfg.ChunkSize, 1024)
	}
}

func TestChunkOverlapEnv(t *testing.T) {
	os.Setenv("CHUNK_OVERLAP", "128")
	defer os.Unsetenv("CHUNK_OVERLAP")
	cfg, err := parseTestFlags([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChunkOverlap != 128 {
		t.Errorf("ChunkOverlap = %d, want %d", cfg.ChunkOverlap, 128)
	}
}

func TestBatchSizeEnv(t *testing.T) {
	os.Setenv("BATCH_SIZE", "50")
	defer os.Unsetenv("BATCH_SIZE")
	cfg, err := parseTestFlags([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BatchSize != 50 {
		t.Errorf("BatchSize = %d, want %d", cfg.BatchSize, 50)
	}
}

func TestLLMBaseURLEnv(t *testing.T) {
	os.Setenv("LLM_BASE_URL", "http://localhost:1234/v1")
	defer os.Unsetenv("LLM_BASE_URL")
	cfg, err := parseTestFlags([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMBaseURL != "http://localhost:1234/v1" {
		t.Errorf("LLMBaseURL = %q, want %q", cfg.LLMBaseURL, "http://localhost:1234/v1")
	}
}

func TestQdrantURLEnv(t *testing.T) {
	os.Setenv("QDRANT_URL", "http://qdrant:6333")
	defer os.Unsetenv("QDRANT_URL")
	cfg, err := parseTestFlags([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.QdrantURL != "http://qdrant:6333" {
		t.Errorf("QdrantURL = %q, want %q", cfg.QdrantURL, "http://qdrant:6333")
	}
}

func TestValidate_ValidWithMinimalFields(t *testing.T) {
	cfg := &Config{
		RepoURL: "https://example.com", RepoPath: "/path", OutputPath: "/out",
		MaxRetries: 0, RetryBackoff: 5 * time.Second, ChunkStrategy: "fixed",
		ChunkSize: 512, ChunkOverlap: 0, EmbeddingModel: "text-embedding-3-small",
		BatchSize: 20, LLMBaseURL: "https://api.openai.com/v1",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_WithFlags(t *testing.T) {
	cfg, err := parseTestFlags([]string{
		"--repo-url", "https://custom.com/repo",
		"--repo-path", "/custom/path",
		"--output", "/custom/output",
		"--max-retries", "5",
		"--retry-backoff", "10s",
		"--log-level", "debug",
		"--chunk-strategy", "semantic",
		"--chunk-size", "256",
		"--chunk-overlap", "32",
		"--embedding-model", "custom-model",
		"--batch-size", "10",
		"--llm-base-url", "http://localhost:8080/v1",
	})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, 5)
	}
	if cfg.RetryBackoff != 10*time.Second {
		t.Errorf("RetryBackoff = %v, want %v", cfg.RetryBackoff, 10*time.Second)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.ChunkStrategy != "semantic" {
		t.Errorf("ChunkStrategy = %q, want %q", cfg.ChunkStrategy, "semantic")
	}
	if cfg.ChunkSize != 256 {
		t.Errorf("ChunkSize = %d, want %d", cfg.ChunkSize, 256)
	}
	if cfg.ChunkOverlap != 32 {
		t.Errorf("ChunkOverlap = %d, want %d", cfg.ChunkOverlap, 32)
	}
	if cfg.EmbeddingModel != "custom-model" {
		t.Errorf("EmbeddingModel = %q, want %q", cfg.EmbeddingModel, "custom-model")
	}
	if cfg.BatchSize != 10 {
		t.Errorf("BatchSize = %d, want %d", cfg.BatchSize, 10)
	}
	if cfg.LLMBaseURL != "http://localhost:8080/v1" {
		t.Errorf("LLMBaseURL = %q, want %q", cfg.LLMBaseURL, "http://localhost:8080/v1")
	}
}
