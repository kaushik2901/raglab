package generator

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

func TestGeneratorFactory_WrapsWithRateLimiter(t *testing.T) {
	os.Setenv("GENERATOR_RATE_LIMIT_RPM", "100")
	os.Setenv("LLM_BASE_URL", "http://localhost:9999")
	os.Setenv("LLM_API_KEY", "test-key")
	defer os.Unsetenv("GENERATOR_RATE_LIMIT_RPM")
	defer os.Unsetenv("LLM_BASE_URL")
	defer os.Unsetenv("LLM_API_KEY")

	g, err := New(config.ProviderOpenAI, "gpt-4o-mini")
	assert.NoError(t, err)
	_, ok := g.(*RateLimitedGenerator)
	assert.True(t, ok, "expected *RateLimitedGenerator when RPM > 0")
}

func TestGeneratorFactory_NoRateLimiter(t *testing.T) {
	os.Setenv("GENERATOR_RATE_LIMIT_RPM", "0")
	os.Setenv("LLM_BASE_URL", "http://localhost:9999")
	os.Setenv("LLM_API_KEY", "test-key")
	defer os.Unsetenv("GENERATOR_RATE_LIMIT_RPM")
	defer os.Unsetenv("LLM_BASE_URL")
	defer os.Unsetenv("LLM_API_KEY")

	g, err := New(config.ProviderOpenAI, "gpt-4o-mini")
	assert.NoError(t, err)
	_, ok := g.(*RateLimitedGenerator)
	assert.False(t, ok, "should NOT be *RateLimitedGenerator when RPM = 0")
}

func TestGeneratorFactory_InvalidRPMFallback(t *testing.T) {
	os.Setenv("GENERATOR_RATE_LIMIT_RPM", "not-a-number")
	os.Setenv("LLM_BASE_URL", "http://localhost:9999")
	os.Setenv("LLM_API_KEY", "test-key")
	defer os.Unsetenv("GENERATOR_RATE_LIMIT_RPM")
	defer os.Unsetenv("LLM_BASE_URL")
	defer os.Unsetenv("LLM_API_KEY")

	g, err := New(config.ProviderOpenAI, "gpt-4o-mini")
	assert.NoError(t, err)
	_, ok := g.(*RateLimitedGenerator)
	assert.True(t, ok, "should wrap on invalid RPM (falls back to default 100)")
}
