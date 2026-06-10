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
	t.Parallel()
	cfg := &Config{
		MaxRetries:   -1,
		RetryBackoff: 5 * time.Second,
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_ZeroMaxRetries(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		MaxRetries:   0,
		RetryBackoff: 5 * time.Second,
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_ZeroRetryBackoff(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		MaxRetries:   3,
		RetryBackoff: 0,
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_NegativeRetryBackoff(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		MaxRetries:   3,
		RetryBackoff: -5 * time.Second,
		LLMBaseURL:   "https://api.openai.com/v1",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_EmptyBaseURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		MaxRetries:   3,
		RetryBackoff: 5 * time.Second,
		LLMBaseURL:   "",
	}
	err := cfg.Validate()
	assert.Error(t, err)
}

func TestValidate_Valid(t *testing.T) {
	t.Parallel()
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
	lookup := func(key string) string {
		if key == "TEST_ENV_KEY" {
			return "from_env"
		}
		return ""
	}
	assert.Equal(t, "from_env", EnvOrDefaultWith(lookup, "TEST_ENV_KEY", "default"))
}

func TestEnvOrDefault_Fallback(t *testing.T) {
	lookup := func(key string) string { return "" }
	assert.Equal(t, "default", EnvOrDefaultWith(lookup, "TEST_ENV_KEY_NONEXISTENT", "default"))
}

func TestEnvOrDefault_EmptyVar(t *testing.T) {
	lookup := func(key string) string { return "" }
	assert.Equal(t, "default", EnvOrDefaultWith(lookup, "TEST_ENV_EMPTY", "default"))
}

func TestIntEnvOrDefault(t *testing.T) {
	lookup := func(key string) string {
		if key == "TEST_INT_KEY" {
			return "42"
		}
		return ""
	}
	assert.Equal(t, 42, IntEnvOrDefaultWith(lookup, "TEST_INT_KEY", 1))
}

func TestIntEnvOrDefault_Fallback(t *testing.T) {
	lookup := func(key string) string { return "" }
	assert.Equal(t, 10, IntEnvOrDefaultWith(lookup, "TEST_INT_NONEXISTENT", 10))
}

func TestIntEnvOrDefault_InvalidValue(t *testing.T) {
	lookup := func(key string) string {
		if key == "TEST_INT_INVALID" {
			return "not-a-number"
		}
		return ""
	}
	assert.Equal(t, 7, IntEnvOrDefaultWith(lookup, "TEST_INT_INVALID", 7))
}

func TestIntEnvOrDefault_EmptyValue(t *testing.T) {
	lookup := func(key string) string { return "" }
	assert.Equal(t, 5, IntEnvOrDefaultWith(lookup, "TEST_INT_EMPTY", 5))
}

func TestDurationEnvOrDefault(t *testing.T) {
	lookup := func(key string) string {
		if key == "TEST_DUR_KEY" {
			return "10s"
		}
		return ""
	}
	assert.Equal(t, 10*time.Second, DurationEnvOrDefaultWith(lookup, "TEST_DUR_KEY", time.Second))
}

func TestDurationEnvOrDefault_Fallback(t *testing.T) {
	lookup := func(key string) string { return "" }
	assert.Equal(t, 30*time.Second, DurationEnvOrDefaultWith(lookup, "TEST_DUR_NONEXISTENT", 30*time.Second))
}

func TestDurationEnvOrDefault_InvalidValue(t *testing.T) {
	lookup := func(key string) string {
		if key == "TEST_DUR_INVALID" {
			return "not-a-duration"
		}
		return ""
	}
	assert.Equal(t, 3*time.Second, DurationEnvOrDefaultWith(lookup, "TEST_DUR_INVALID", 3*time.Second))
}

func TestDurationEnvOrDefault_EmptyValue(t *testing.T) {
	lookup := func(key string) string { return "" }
	assert.Equal(t, 2*time.Second, DurationEnvOrDefaultWith(lookup, "TEST_DUR_EMPTY", 2*time.Second))
}

func TestFloatEnvOrDefault(t *testing.T) {
	lookup := func(key string) string {
		if key == "TEST_FLOAT_KEY" {
			return "42.5"
		}
		return ""
	}
	val := FloatEnvOrDefaultWith(lookup, "TEST_FLOAT_KEY", 10.0)
	assert.InDelta(t, 42.5, val, 0.0001)
}

func TestFloatEnvOrDefault_Default(t *testing.T) {
	lookup := func(key string) string { return "" }
	val := FloatEnvOrDefaultWith(lookup, "TEST_FLOAT_KEY_MISSING", 10.0)
	assert.InDelta(t, 10.0, val, 0.0001)
}

func TestFloatEnvOrDefault_Invalid(t *testing.T) {
	lookup := func(key string) string {
		if key == "TEST_FLOAT_KEY_INVALID" {
			return "not-a-number"
		}
		return ""
	}
	val := FloatEnvOrDefaultWith(lookup, "TEST_FLOAT_KEY_INVALID", 10.0)
	assert.InDelta(t, 10.0, val, 0.0001, "should fall back to default on parse error")
}

func TestFloatEnvOrDefault_Empty(t *testing.T) {
	lookup := func(key string) string { return "" }
	val := FloatEnvOrDefaultWith(lookup, "TEST_FLOAT_KEY_EMPTY", 10.0)
	assert.InDelta(t, 10.0, val, 0.0001, "should fall back to default on empty string")
}

func TestBackwardCompat_EnvOrDefault(t *testing.T) {
	os.Setenv("TEST_BC_ENV", "from_real_env")
	defer os.Unsetenv("TEST_BC_ENV")
	assert.Equal(t, "from_real_env", EnvOrDefault("TEST_BC_ENV", "default"))
}

func TestBackwardCompat_IntEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_BC_INT", "99")
	defer os.Unsetenv("TEST_BC_INT")
	assert.Equal(t, 99, IntEnvOrDefault("TEST_BC_INT", 1))
}

func TestBackwardCompat_DurationEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_BC_DUR", "30s")
	defer os.Unsetenv("TEST_BC_DUR")
	assert.Equal(t, 30*time.Second, DurationEnvOrDefault("TEST_BC_DUR", time.Second))
}

func TestBackwardCompat_FloatEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_BC_FLOAT", "3.14")
	defer os.Unsetenv("TEST_BC_FLOAT")
	assert.InDelta(t, 3.14, FloatEnvOrDefault("TEST_BC_FLOAT", 1.0), 0.0001)
}

func TestBackwardCompat_ResolveProviderConfig(t *testing.T) {
	os.Setenv("OPENAI_BASE_URL", "https://custom.openai.com")
	os.Setenv("OPENAI_API_KEY", "sk-custom")
	defer os.Unsetenv("OPENAI_BASE_URL")
	defer os.Unsetenv("OPENAI_API_KEY")
	url, key := ResolveProviderConfig(ProviderOpenAI)
	assert.Equal(t, "https://custom.openai.com", url)
	assert.Equal(t, "sk-custom", key)
}

func TestLoadWithEnv_FakeEnv(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	lookup := func(key string) string {
		switch key {
		case "LLM_BASE_URL":
			return "https://fake.openai.com"
		case "QDRANT_URL":
			return "http://fake-qdrant:6334"
		case "DATABASE_URL":
			return "postgres://fake:fake@localhost:5432/fake"
		default:
			return ""
		}
	}

	cfg, err := LoadWithEnv(fs, lookup, []string{})
	require.NoError(t, err)

	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 5*time.Second, cfg.RetryBackoff)
	assert.Equal(t, "https://fake.openai.com", cfg.LLMBaseURL)
	assert.Equal(t, "http://fake-qdrant:6334", cfg.QdrantURL)
	assert.Equal(t, "postgres://fake:fake@localhost:5432/fake", cfg.DatabaseURL)
	assert.Empty(t, cfg.LLMApiKey)
	assert.Empty(t, cfg.QdrantAPIKey)
}

func TestLoadWithEnv_WithFlags(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	lookup := func(key string) string {
		if key == "LLM_BASE_URL" {
			return "https://example.com"
		}
		return ""
	}

	cfg, err := LoadWithEnv(fs, lookup, []string{
		"--max-retries", "5",
		"--retry-backoff", "10s",
		"--log-level", "debug",
	})
	require.NoError(t, err)

	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 10*time.Second, cfg.RetryBackoff)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "https://example.com", cfg.LLMBaseURL)
}

func TestResolveProviderConfigWith_OpenAI(t *testing.T) {
	lookup := func(key string) string {
		switch key {
		case "OPENAI_BASE_URL":
			return "https://custom.openai.com"
		case "OPENAI_API_KEY":
			return "sk-custom-key"
		default:
			return ""
		}
	}
	url, key := ResolveProviderConfigWith(lookup, ProviderOpenAI)
	assert.Equal(t, "https://custom.openai.com", url)
	assert.Equal(t, "sk-custom-key", key)
}

func TestResolveProviderConfigWith_OpenAIFallback(t *testing.T) {
	lookup := func(key string) string {
		switch key {
		case "LLM_BASE_URL":
			return "https://legacy.openai.com"
		case "LLM_API_KEY":
			return "sk-legacy"
		default:
			return ""
		}
	}
	url, key := ResolveProviderConfigWith(lookup, ProviderOpenAI)
	assert.Equal(t, "https://legacy.openai.com", url)
	assert.Equal(t, "sk-legacy", key)
}

func TestResolveProviderConfigWith_OpenRouter(t *testing.T) {
	lookup := func(key string) string {
		if key == "OPENROUTER_API_KEY" {
			return "sk-or-key"
		}
		return ""
	}
	url, key := ResolveProviderConfigWith(lookup, ProviderOpenRouter)
	assert.Contains(t, url, "openrouter")
	assert.Equal(t, "sk-or-key", key)
}

func TestResolveProviderConfigWith_LMStudio(t *testing.T) {
	lookup := func(key string) string { return "" }
	url, key := ResolveProviderConfigWith(lookup, ProviderLMStudio)
	assert.Contains(t, url, "localhost")
	assert.Empty(t, key)
}

func TestResolveProviderConfigWith_Gemini(t *testing.T) {
	lookup := func(key string) string {
		if key == "GEMINI_API_KEY" {
			return "sk-gemini-key"
		}
		return ""
	}
	url, key := ResolveProviderConfigWith(lookup, ProviderGemini)
	assert.Contains(t, url, "googleapis")
	assert.Equal(t, "sk-gemini-key", key)
}

func TestResolveProviderConfigWith_Default(t *testing.T) {
	lookup := func(key string) string { return "" }
	url, key := ResolveProviderConfigWith(lookup, Provider("unknown"))
	assert.Contains(t, url, "openai")
	assert.Empty(t, key)
}
