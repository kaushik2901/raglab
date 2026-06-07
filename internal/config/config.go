package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderGemini     Provider = "gemini"
	ProviderOpenRouter Provider = "openrouter"
	ProviderLMStudio   Provider = "lmstudio"
)

type Config struct {
	MaxRetries   int
	RetryBackoff time.Duration
	LogLevel     string
	DatabaseURL  string

	LLMBaseURL   string
	LLMApiKey    string
	QdrantURL    string
	QdrantAPIKey string
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.IntVar(&cfg.MaxRetries, "max-retries", IntEnvOrDefault("MAX_RETRIES", 3), "Maximum retry count for stages")
	flag.DurationVar(&cfg.RetryBackoff, "retry-backoff", DurationEnvOrDefault("RETRY_BACKOFF", 5*time.Second), "Retry backoff duration")
	flag.StringVar(&cfg.LogLevel, "log-level", EnvOrDefault("LOG_LEVEL", "info"), "Log level (debug/info/warn)")

	flag.Parse()

	cfg.LLMApiKey = os.Getenv("LLM_API_KEY")
	cfg.QdrantAPIKey = os.Getenv("QDRANT_API_KEY")

	cfg.LLMBaseURL = EnvOrDefault("LLM_BASE_URL", "https://api.openai.com/v1")
	cfg.QdrantURL = EnvOrDefault("QDRANT_URL", "http://localhost:6334")
	cfg.DatabaseURL = EnvOrDefault("DATABASE_URL", "postgres://rag:rag@localhost:5432/rag?sslmode=disable")

	return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	if c.MaxRetries < 0 {
		return errors.New("max-retries must be non-negative")
	}
	if c.RetryBackoff <= 0 {
		return errors.New("retry-backoff must be positive")
	}
	if c.LLMBaseURL == "" {
		return errors.New("llm-base-url is required")
	}
	return nil
}

// ResolveProviderConfig returns the base URL and API key for a given provider.
// Each provider has its own env var convention. The "openai" provider falls back
// to the legacy LLM_BASE_URL / LLM_API_KEY env vars for backward compatibility.
func ResolveProviderConfig(p Provider) (baseURL, apiKey string) {
	switch p {
	case ProviderOpenAI:
		return resolveWithFallback("OPENAI_BASE_URL", "LLM_BASE_URL", "https://api.openai.com/v1"),
			resolveWithFallback("OPENAI_API_KEY", "LLM_API_KEY", "")
	case ProviderOpenRouter:
		return EnvOrDefault("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
			os.Getenv("OPENROUTER_API_KEY")
	case ProviderLMStudio:
		return EnvOrDefault("LMSTUDIO_BASE_URL", "http://localhost:1234/v1"), ""
	case ProviderGemini:
		return EnvOrDefault("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai"),
			os.Getenv("GEMINI_API_KEY")
	default:
		return EnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			os.Getenv("OPENAI_API_KEY")
	}
}

func resolveWithFallback(primary, fallback, defaultVal string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	return EnvOrDefault(fallback, defaultVal)
}

func EnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func IntEnvOrDefault(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func DurationEnvOrDefault(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}

func ResolveTag(tag, prefix string) string {
	if tag != "" {
		return tag
	}
	return fmt.Sprintf("%s-%s", prefix, time.Now().Format("20060102-150405"))
}


