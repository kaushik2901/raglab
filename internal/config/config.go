package config

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"time"
)

type Config struct {
	RepoURL      string
	RepoPath     string
	OutputPath   string
	MaxRetries   int
	RetryBackoff time.Duration
	LogLevel     string

	ChunkStrategy  string
	ChunkSize      int
	ChunkOverlap   int
	EmbeddingModel string
	BatchSize      int

	LLMBaseURL   string
	LLMApiKey    string
	QdrantURL    string
	QdrantAPIKey string
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.RepoURL, "repo-url", envOrDefault("REPO_URL", "https://gitlab.com/gitlab-com/content-sites/handbook"), "Repository URL to clone")
	flag.StringVar(&cfg.RepoPath, "repo-path", envOrDefault("REPO_PATH", "./handbook"), "Local path for repository clone")
	flag.StringVar(&cfg.OutputPath, "output", envOrDefault("OUTPUT_PATH", "./output"), "Output directory for cleaned markdown")
	flag.IntVar(&cfg.MaxRetries, "max-retries", intEnvOrDefault("MAX_RETRIES", 3), "Maximum retry count for stages")
	flag.DurationVar(&cfg.RetryBackoff, "retry-backoff", durationEnvOrDefault("RETRY_BACKOFF", 5*time.Second), "Retry backoff duration")
	flag.StringVar(&cfg.LogLevel, "log-level", envOrDefault("LOG_LEVEL", "info"), "Log level (debug/info/warn)")

	flag.StringVar(&cfg.ChunkStrategy, "chunk-strategy", envOrDefault("CHUNK_STRATEGY", "fixed"), "Chunking strategy (fixed only)")
	flag.IntVar(&cfg.ChunkSize, "chunk-size", intEnvOrDefault("CHUNK_SIZE", 512), "Target token count per chunk")
	flag.IntVar(&cfg.ChunkOverlap, "chunk-overlap", intEnvOrDefault("CHUNK_OVERLAP", 64), "Token overlap between chunks")
	flag.StringVar(&cfg.EmbeddingModel, "embedding-model", envOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"), "Embedding model name")
	flag.IntVar(&cfg.BatchSize, "batch-size", intEnvOrDefault("BATCH_SIZE", 20), "Embedding batch size")
	flag.StringVar(&cfg.LLMBaseURL, "llm-base-url", envOrDefault("LLM_BASE_URL", "https://api.openai.com/v1"), "LLM base URL (OpenAI-compatible)")
	flag.Parse()

	cfg.LLMApiKey = os.Getenv("LLM_API_KEY")
	cfg.QdrantURL = envOrDefault("QDRANT_URL", "http://localhost:6333")
	cfg.QdrantAPIKey = os.Getenv("QDRANT_API_KEY")

	return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	if c.RepoURL == "" {
		return errors.New("repo-url is required")
	}
	if c.RepoPath == "" {
		return errors.New("repo-path is required")
	}
	if c.OutputPath == "" {
		return errors.New("output-path is required")
	}
	if c.MaxRetries < 0 {
		return errors.New("max-retries must be non-negative")
	}
	if c.RetryBackoff <= 0 {
		return errors.New("retry-backoff must be positive")
	}
	if c.ChunkStrategy != "fixed" {
		return errors.New("chunk-strategy must be 'fixed'")
	}
	if c.ChunkSize <= 0 {
		return errors.New("chunk-size must be positive")
	}
	if c.ChunkOverlap < 0 {
		return errors.New("chunk-overlap must be non-negative")
	}
	if c.ChunkOverlap >= c.ChunkSize {
		return errors.New("chunk-overlap must be less than chunk-size")
	}
	if c.EmbeddingModel == "" {
		return errors.New("embedding-model is required")
	}
	if c.BatchSize <= 0 {
		return errors.New("batch-size must be positive")
	}
	if c.LLMBaseURL == "" {
		return errors.New("llm-base-url is required")
	}
	return nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func intEnvOrDefault(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func durationEnvOrDefault(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
