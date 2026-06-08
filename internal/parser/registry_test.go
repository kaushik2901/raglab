package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParserRegistry_Valid(t *testing.T) {
	p, err := New("markdown")
	assert.NoError(t, err)
	assert.NotNil(t, p)
	_, ok := p.(*MarkdownParser)
	assert.True(t, ok, "expected *MarkdownParser")
}

func TestParserRegistry_Invalid(t *testing.T) {
	_, err := New("nonexistent")
	assert.Error(t, err)
}

func TestParserRegistry_Default(t *testing.T) {
	assert.NotNil(t, Default)
	_, ok := Default.(*MarkdownParser)
	assert.True(t, ok, "Default should be *MarkdownParser")
}

func TestParserRegistry_Register(t *testing.T) {
	called := false
	RegisterParser("test", func() Parser {
		called = true
		return &MarkdownParser{}
	})
	p, err := New("test")
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.True(t, called)
}
