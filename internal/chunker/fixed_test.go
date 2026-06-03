package chunker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func words(n int) string {
	ws := make([]string, n)
	for i := range ws {
		ws[i] = "word"
	}
	return strings.Join(ws, " ")
}

func TestFixedChunker_Basic(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(100)}
	c := NewFixedChunker(30, 10)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)

	step := 20
	expected := (100 + step - 1) / step
	assert.Len(t, chunks, expected)

	var totalWords int
	for _, ch := range chunks {
		totalWords += len(strings.Fields(ch.Content))
	}
	assert.GreaterOrEqual(t, totalWords, 100)
}

func TestFixedChunker_NoOverlap(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(100)}
	c := NewFixedChunker(30, 0)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)

	for _, ch := range chunks {
		wc := len(strings.Fields(ch.Content))
		assert.LessOrEqual(t, wc, 30)
	}
}

func TestFixedChunker_FullOverlap(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(50)}
	c := NewFixedChunker(10, 20)
	assert.Equal(t, 9, c.Overlap, "Overlap should be clamped to size-1")

	chunks, err := c.Chunk(doc)
	require.NoError(t, err)
	assert.NotEmpty(t, chunks)
}

func TestFixedChunker_EmptyDoc(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: ""}
	c := NewFixedChunker(10, 2)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestFixedChunker_ShortDoc(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(3)}
	c := NewFixedChunker(100, 10)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)
	assert.Len(t, chunks, 1)
}

func TestFixedChunker_ExactMultiple(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(100)}
	c := NewFixedChunker(50, 0)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)
	assert.Len(t, chunks, 2)

	wc1 := len(strings.Fields(chunks[0].Content))
	wc2 := len(strings.Fields(chunks[1].Content))
	assert.Equal(t, 50, wc1)
	assert.Equal(t, 50, wc2)
}

func TestFixedChunker_SingleWord(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: "hello"}
	c := NewFixedChunker(10, 2)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)
	assert.Len(t, chunks, 1)
}

func TestFixedChunker_WhitespaceOnly(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: "   \n  \t  "}
	c := NewFixedChunker(10, 2)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestFixedChunker_ChunkIDs(t *testing.T) {
	doc := types.Document{Path: "docs/page.md", Content: words(80)}
	c := NewFixedChunker(30, 5)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)

	if len(chunks) > 0 {
		assert.Equal(t, "docs/page.md-chunk-0000", chunks[0].ID)
	}
	if len(chunks) > 1 {
		assert.Equal(t, "docs/page.md-chunk-0001", chunks[1].ID)
	}
}

func TestFixedChunker_DocumentPath(t *testing.T) {
	doc := types.Document{Path: "foo/bar.md", Content: words(20)}
	c := NewFixedChunker(10, 0)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)

	for _, ch := range chunks {
		assert.Equal(t, "foo/bar.md", ch.DocumentPath)
	}
}

func TestFixedChunker_TokenCount(t *testing.T) {
	content := words(40)
	doc := types.Document{Path: "doc.md", Content: content}
	c := NewFixedChunker(10, 0)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)

	for _, ch := range chunks {
		expected := len(ch.Content) / 4
		assert.Equal(t, expected, ch.TokenCount)
	}
}

func TestFixedChunker_MetadataNil(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(5)}
	c := NewFixedChunker(10, 0)
	chunks, err := c.Chunk(doc)
	require.NoError(t, err)

	for _, ch := range chunks {
		assert.Nil(t, ch.Metadata)
	}
}
