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

	APIRequestTimeout time.Duration
	ChatMemorySize    int
	ArtifactsDir      string
}

// EnvLookup abstracts environment variable lookup for testability.
type EnvLookup func(key string) string

// Load is the backward-compatible entry point used by cmd/*/main.go.
// Uses a private FlagSet and os.Getenv — does NOT pollute global flag state.
func Load() (*Config, error) {
	return LoadWithEnv(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Getenv, nil)
}

// LoadWithEnv loads config using an explicit FlagSet and EnvLookup.
// args specifies the command-line arguments to parse; nil means use os.Args[1:].
// This is the testable version — no global flag pollution, no direct os.Getenv.
func LoadWithEnv(flagSet *flag.FlagSet, lookup EnvLookup, args []string) (*Config, error) {
	cfg := &Config{}

	flagSet.IntVar(&cfg.MaxRetries, "max-retries", IntEnvOrDefaultWith(lookup, "MAX_RETRIES", 3), "Maximum retry count for stages")
	flagSet.DurationVar(&cfg.RetryBackoff, "retry-backoff", DurationEnvOrDefaultWith(lookup, "RETRY_BACKOFF", 5*time.Second), "Retry backoff duration")
	flagSet.StringVar(&cfg.LogLevel, "log-level", EnvOrDefaultWith(lookup, "LOG_LEVEL", "info"), "Log level (debug/info/warn)")

	if args == nil {
		args = os.Args[1:]
	}
	flagSet.Parse(args)

	cfg.LLMApiKey = lookup("LLM_API_KEY")
	cfg.QdrantAPIKey = lookup("QDRANT_API_KEY")

	cfg.LLMBaseURL = EnvOrDefaultWith(lookup, "LLM_BASE_URL", "https://api.openai.com")
	cfg.QdrantURL = EnvOrDefaultWith(lookup, "QDRANT_URL", "http://localhost:6334")
	cfg.DatabaseURL = EnvOrDefaultWith(lookup, "DATABASE_URL", "postgres://rag:rag@localhost:5432/rag?sslmode=disable")

	cfg.APIRequestTimeout = DurationEnvOrDefaultWith(lookup, "API_REQUEST_TIMEOUT", 60*time.Second)
	cfg.ChatMemorySize = IntEnvOrDefaultWith(lookup, "CHAT_MEMORY_SIZE", 10)
	cfg.ArtifactsDir = EnvOrDefaultWith(lookup, "ARTIFACTS_DIR", "artifacts")

	return cfg, cfg.Validate()
}

func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{MaxRetries=%d, RetryBackoff=%s, LogLevel=%s, DatabaseURL=%s, LLMBaseURL=%s, LLMApiKey=%s, QdrantURL=%s, QdrantAPIKey=%s}",
		c.MaxRetries, c.RetryBackoff, c.LogLevel, c.DatabaseURL, c.LLMBaseURL,
		redact(c.LLMApiKey), c.QdrantURL, redact(c.QdrantAPIKey),
	)
}

func redact(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
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
	return ResolveProviderConfigWith(os.Getenv, p)
}

// ResolveProviderConfigWith is the injectable-env variant of ResolveProviderConfig.
func ResolveProviderConfigWith(lookup EnvLookup, p Provider) (baseURL, apiKey string) {
	switch p {
	case ProviderOpenAI:
		return resolveWithFallback(lookup, "OPENAI_BASE_URL", "LLM_BASE_URL", "https://api.openai.com"),
			resolveWithFallback(lookup, "OPENAI_API_KEY", "LLM_API_KEY", "")
	case ProviderOpenRouter:
		return EnvOrDefaultWith(lookup, "OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
			lookup("OPENROUTER_API_KEY")
	case ProviderLMStudio:
		return EnvOrDefaultWith(lookup, "LMSTUDIO_BASE_URL", "http://localhost:1234/v1"), ""
	case ProviderGemini:
		return EnvOrDefaultWith(lookup, "GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai"),
			lookup("GEMINI_API_KEY")
	default:
		return EnvOrDefaultWith(lookup, "OPENAI_BASE_URL", "https://api.openai.com"),
			lookup("OPENAI_API_KEY")
	}
}

func resolveWithFallback(lookup EnvLookup, primary, fallback, defaultVal string) string {
	if v := lookup(primary); v != "" {
		return v
	}
	return EnvOrDefaultWith(lookup, fallback, defaultVal)
}

// EnvOrDefaultWith looks up a key via the provided EnvLookup function.
func EnvOrDefaultWith(lookup EnvLookup, key, defaultVal string) string {
	if v := lookup(key); v != "" {
		return v
	}
	return defaultVal
}

// EnvOrDefault is the backward-compatible wrapper using os.Getenv.
func EnvOrDefault(key, defaultVal string) string {
	return EnvOrDefaultWith(os.Getenv, key, defaultVal)
}

// IntEnvOrDefaultWith looks up and parses an int env var via lookup.
func IntEnvOrDefaultWith(lookup EnvLookup, key string, defaultVal int) int {
	if v := lookup(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func IntEnvOrDefault(key string, defaultVal int) int {
	return IntEnvOrDefaultWith(os.Getenv, key, defaultVal)
}

// DurationEnvOrDefaultWith looks up and parses a duration env var via lookup.
func DurationEnvOrDefaultWith(lookup EnvLookup, key string, defaultVal time.Duration) time.Duration {
	if v := lookup(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}

func DurationEnvOrDefault(key string, defaultVal time.Duration) time.Duration {
	return DurationEnvOrDefaultWith(os.Getenv, key, defaultVal)
}

// FloatEnvOrDefaultWith looks up and parses a float64 env var via lookup.
func FloatEnvOrDefaultWith(lookup EnvLookup, key string, defaultVal float64) float64 {
	if v := lookup(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func FloatEnvOrDefault(key string, defaultVal float64) float64 {
	return FloatEnvOrDefaultWith(os.Getenv, key, defaultVal)
}
