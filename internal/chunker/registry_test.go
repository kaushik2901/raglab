package chunker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkerRegistry_Valid(t *testing.T) {
	c, err := New("fixed", map[string]any{"size": 100, "overlap": 20})
	assert.NoError(t, err)
	assert.NotNil(t, c)
	fc, ok := c.(*FixedChunker)
	assert.True(t, ok, "expected *FixedChunker")
	assert.Equal(t, 100, fc.Size)
	assert.Equal(t, 20, fc.Overlap)
}

func TestChunkerRegistry_Invalid(t *testing.T) {
	_, err := New("nonexistent", map[string]any{"size": 100})
	assert.Error(t, err)
}

func TestChunkerRegistry_Register(t *testing.T) {
	called := false
	RegisterChunker("test", func(cfg map[string]any) (Chunker, error) {
		called = true
		return NewFixedChunker(getInt(cfg, "size", 50), getInt(cfg, "overlap", 10)), nil
	})
	c, err := New("test", map[string]any{"size": 50, "overlap": 10})
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.True(t, called)
}
