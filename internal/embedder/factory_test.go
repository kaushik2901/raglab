package embedder

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaushik2901/raglab/internal/config"
)

func TestFactory_WrapsWithRateLimiter(t *testing.T) {
	os.Setenv("EMBEDDER_RATE_LIMIT_RPM", "100")
	os.Setenv("LLM_BASE_URL", "http://localhost:9999")
	os.Setenv("LLM_API_KEY", "test-key")
	defer os.Unsetenv("EMBEDDER_RATE_LIMIT_RPM")
	defer os.Unsetenv("LLM_BASE_URL")
	defer os.Unsetenv("LLM_API_KEY")

	e, err := New(config.ProviderOpenAI, "text-embedding-3-small", 20)
	assert.NoError(t, err)
	_, ok := e.(*RateLimitedEmbedder)
	assert.True(t, ok, "expected *RateLimitedEmbedder when RPM > 0")
}

func TestFactory_NoRateLimiter(t *testing.T) {
	os.Setenv("EMBEDDER_RATE_LIMIT_RPM", "0")
	os.Setenv("LLM_BASE_URL", "http://localhost:9999")
	os.Setenv("LLM_API_KEY", "test-key")
	defer os.Unsetenv("EMBEDDER_RATE_LIMIT_RPM")
	defer os.Unsetenv("LLM_BASE_URL")
	defer os.Unsetenv("LLM_API_KEY")

	e, err := New(config.ProviderOpenAI, "text-embedding-3-small", 20)
	assert.NoError(t, err)
	_, ok := e.(*RateLimitedEmbedder)
	assert.False(t, ok, "should NOT be *RateLimitedEmbedder when RPM = 0")
}

func TestFactory_InvalidRPMFallback(t *testing.T) {
	os.Setenv("EMBEDDER_RATE_LIMIT_RPM", "not-a-number")
	os.Setenv("LLM_BASE_URL", "http://localhost:9999")
	os.Setenv("LLM_API_KEY", "test-key")
	defer os.Unsetenv("EMBEDDER_RATE_LIMIT_RPM")
	defer os.Unsetenv("LLM_BASE_URL")
	defer os.Unsetenv("LLM_API_KEY")

	e, err := New(config.ProviderOpenAI, "text-embedding-3-small", 20)
	assert.NoError(t, err)
	_, ok := e.(*RateLimitedEmbedder)
	assert.True(t, ok, "should wrap with rate limiter on invalid RPM (falls back to default 100)")
}
