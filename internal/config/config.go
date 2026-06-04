package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
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

	flag.IntVar(&cfg.MaxRetries, "max-retries", intEnvOrDefault("MAX_RETRIES", 3), "Maximum retry count for stages")
	flag.DurationVar(&cfg.RetryBackoff, "retry-backoff", durationEnvOrDefault("RETRY_BACKOFF", 5*time.Second), "Retry backoff duration")
	flag.StringVar(&cfg.LogLevel, "log-level", envOrDefault("LOG_LEVEL", "info"), "Log level (debug/info/warn)")

	flag.Parse()

	cfg.LLMApiKey = os.Getenv("LLM_API_KEY")
	cfg.QdrantAPIKey = os.Getenv("QDRANT_API_KEY")

	cfg.LLMBaseURL = envOrDefault("LLM_BASE_URL", "https://api.openai.com/v1")
	cfg.QdrantURL = envOrDefault("QDRANT_URL", "http://localhost:6334")
	cfg.DatabaseURL = envOrDefault("DATABASE_URL", "postgres://rag:rag@localhost:5432/rag?sslmode=disable")

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
