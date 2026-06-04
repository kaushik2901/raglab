package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	g := New("https://api.openai.com/v1", "sk-test", "gpt-4o-mini")
	assert.NotNil(t, g)
	assert.Equal(t, "gpt-4o-mini", g.model)
}

func TestNewEmptyAPIKey(t *testing.T) {
	g := New("http://localhost:1234/v1", "", "local-model")
	assert.NotNil(t, g)
	assert.Equal(t, "local-model", g.model)
}
