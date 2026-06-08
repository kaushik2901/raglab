package chunker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkerRegistry_Valid(t *testing.T) {
	c, err := New("fixed", 100, 20)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	fc, ok := c.(*FixedChunker)
	assert.True(t, ok, "expected *FixedChunker")
	assert.Equal(t, 100, fc.Size)
	assert.Equal(t, 20, fc.Overlap)
}

func TestChunkerRegistry_Invalid(t *testing.T) {
	_, err := New("nonexistent", 100, 20)
	assert.Error(t, err)
}

func TestChunkerRegistry_Register(t *testing.T) {
	called := false
	RegisterChunker("test", func(size, overlap int) Chunker {
		called = true
		return NewFixedChunker(size, overlap)
	})
	c, err := New("test", 50, 10)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.True(t, called)
}
