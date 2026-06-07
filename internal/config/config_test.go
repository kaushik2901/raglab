package config

import (
	"flag"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_NegativeMaxRetries(t *testing.T) {
	cfg := &Config{
		MaxRetries:   -1,
		RetryBackoff: 5 * time.Second,
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_ZeroMaxRetries(t *testing.T) {
	cfg := &Config{
		MaxRetries:   0,
		RetryBackoff: 5 * time.Second,
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_ZeroRetryBackoff(t *testing.T) {
	cfg := &Config{
		MaxRetries:   3,
		RetryBackoff: 0,
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_NegativeRetryBackoff(t *testing.T) {
	cfg := &Config{
		MaxRetries:   3,
		RetryBackoff: -5 * time.Second,
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_EmptyBaseURL(t *testing.T) {
	cfg := &Config{
		MaxRetries:   3,
		RetryBackoff: 5 * time.Second,
		LLMBaseURL:   "",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		MaxRetries:   3,
		RetryBackoff: 5 * time.Second,
		LogLevel:     "info",
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_ENV_KEY", "from_env")
	defer os.Unsetenv("TEST_ENV_KEY")

	assert.Equal(t, "from_env", EnvOrDefault("TEST_ENV_KEY", "default"))
}

func TestEnvOrDefault_Fallback(t *testing.T) {
	assert.Equal(t, "default", EnvOrDefault("TEST_ENV_KEY_NONEXISTENT", "default"))
}

func TestEnvOrDefault_EmptyVar(t *testing.T) {
	os.Setenv("TEST_ENV_EMPTY", "")
	defer os.Unsetenv("TEST_ENV_EMPTY")

	assert.Equal(t, "default", EnvOrDefault("TEST_ENV_EMPTY", "default"))
}

func TestIntEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_INT_KEY", "42")
	defer os.Unsetenv("TEST_INT_KEY")

	assert.Equal(t, 42, IntEnvOrDefault("TEST_INT_KEY", 1))
}

func TestIntEnvOrDefault_Fallback(t *testing.T) {
	assert.Equal(t, 10, IntEnvOrDefault("TEST_INT_NONEXISTENT", 10))
}

func TestIntEnvOrDefault_InvalidValue(t *testing.T) {
	os.Setenv("TEST_INT_INVALID", "not-a-number")
	defer os.Unsetenv("TEST_INT_INVALID")

	assert.Equal(t, 7, IntEnvOrDefault("TEST_INT_INVALID", 7))
}

func TestIntEnvOrDefault_EmptyValue(t *testing.T) {
	os.Setenv("TEST_INT_EMPTY", "")
	defer os.Unsetenv("TEST_INT_EMPTY")

	assert.Equal(t, 5, IntEnvOrDefault("TEST_INT_EMPTY", 5))
}

func TestDurationEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_DUR_KEY", "10s")
	defer os.Unsetenv("TEST_DUR_KEY")

	assert.Equal(t, 10*time.Second, DurationEnvOrDefault("TEST_DUR_KEY", time.Second))
}

func TestDurationEnvOrDefault_Fallback(t *testing.T) {
	assert.Equal(t, 30*time.Second, DurationEnvOrDefault("TEST_DUR_NONEXISTENT", 30*time.Second))
}

func TestDurationEnvOrDefault_InvalidValue(t *testing.T) {
	os.Setenv("TEST_DUR_INVALID", "not-a-duration")
	defer os.Unsetenv("TEST_DUR_INVALID")

	assert.Equal(t, 3*time.Second, DurationEnvOrDefault("TEST_DUR_INVALID", 3*time.Second))
}

func TestDurationEnvOrDefault_EmptyValue(t *testing.T) {
	os.Setenv("TEST_DUR_EMPTY", "")
	defer os.Unsetenv("TEST_DUR_EMPTY")

	assert.Equal(t, 2*time.Second, DurationEnvOrDefault("TEST_DUR_EMPTY", 2*time.Second))
}

func parseTestFlags(args []string) (*Config, error) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := &Config{}

	fs.IntVar(&cfg.MaxRetries, "max-retries", 3, "")
	fs.DurationVar(&cfg.RetryBackoff, "retry-backoff", 5*time.Second, "")
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg.LLMApiKey = os.Getenv("LLM_API_KEY")
	cfg.QdrantAPIKey = os.Getenv("QDRANT_API_KEY")
	cfg.LLMBaseURL = EnvOrDefault("LLM_BASE_URL", "https://api.openai.com/v1")
	cfg.QdrantURL = EnvOrDefault("QDRANT_URL", "http://localhost:6334")
	cfg.DatabaseURL = EnvOrDefault("DATABASE_URL", "postgres://rag:rag@localhost:5432/rag?sslmode=disable")

	return cfg, cfg.Validate()
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := parseTestFlags([]string{})
	require.NoError(t, err)

	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 5*time.Second, cfg.RetryBackoff)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestResolveTag_Provided(t *testing.T) {
	tag := ResolveTag("my-custom-tag", "idx")
	assert.Equal(t, "my-custom-tag", tag)
}

func TestResolveTag_Generated(t *testing.T) {
	tag := ResolveTag("", "eval")
	assert.Contains(t, tag, "eval-")
	assert.Len(t, tag, len("eval-")+15)
}

func TestLoad_WithFlags(t *testing.T) {
	cfg, err := parseTestFlags([]string{
		"--max-retries", "5",
		"--retry-backoff", "10s",
		"--log-level", "debug",
	})
	require.NoError(t, err)

	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 10*time.Second, cfg.RetryBackoff)
	assert.Equal(t, "debug", cfg.LogLevel)
}
