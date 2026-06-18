package chunker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/raglab/internal/types"
)

type sliceElementReader struct {
	elems []types.Element
	pos   int
	path  string
}

func (r *sliceElementReader) ReadElement() (types.Element, error) {
	if r.pos >= len(r.elems) {
		return types.Element{}, io.EOF
	}
	e := r.elems[r.pos]
	r.pos++
	return e, nil
}

func (r *sliceElementReader) Path() string {
	return r.path
}

func (r *sliceElementReader) Close() error {
	return nil
}

func (r *sliceElementReader) Metadata() map[string]string {
	return nil
}

func elementReaderFromText(path, text string) types.ElementReader {
	return &sliceElementReader{
		elems: []types.Element{{Kind: types.ElementParagraph, Text: text}},
		path:  path,
	}
}

func elementReaderFromElements(path string, elems ...types.Element) types.ElementReader {
	return &sliceElementReader{
		elems: elems,
		path:  path,
	}
}

func words(n int) string {
	ws := make([]string, n)
	for i := range ws {
		ws[i] = "word"
	}
	return strings.Join(ws, " ")
}

func collectChunks(t *testing.T, ctx context.Context, c Chunker, reader types.ElementReader, docPath string) []types.Chunk {
	t.Helper()
	chunkCh, errCh := c.Chunk(ctx, reader, docPath)
	var chunks []types.Chunk
	for ch := range chunkCh {
		chunks = append(chunks, ch)
	}
	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}
	return chunks
}

func TestFixedChunker_Basic(t *testing.T) {
	reader := elementReaderFromText("doc.md", words(100))
	c := NewFixedChunker(30, 10)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")

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
	reader := elementReaderFromText("doc.md", words(100))
	c := NewFixedChunker(30, 0)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")

	for _, ch := range chunks {
		wc := len(strings.Fields(ch.Content))
		assert.LessOrEqual(t, wc, 30)
	}
}

func TestFixedChunker_FullOverlap(t *testing.T) {
	reader := elementReaderFromText("doc.md", words(50))
	c := NewFixedChunker(10, 20)
	assert.Equal(t, 9, c.Overlap, "Overlap should be clamped to size-1")

	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")
	assert.NotEmpty(t, chunks)
}

func TestFixedChunker_EmptyDoc(t *testing.T) {
	reader := elementReaderFromText("doc.md", "")
	c := NewFixedChunker(10, 2)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")
	assert.Empty(t, chunks)
}

func TestFixedChunker_ShortDoc(t *testing.T) {
	reader := elementReaderFromText("doc.md", words(3))
	c := NewFixedChunker(100, 10)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")
	assert.Len(t, chunks, 1)
}

func TestFixedChunker_SingleWord(t *testing.T) {
	reader := elementReaderFromText("doc.md", "hello")
	c := NewFixedChunker(10, 2)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")
	require.Len(t, chunks, 1)
	assert.Equal(t, "hello", chunks[0].Content)
}

func TestFixedChunker_WhitespaceOnly(t *testing.T) {
	reader := elementReaderFromText("doc.md", "   \n  \t  ")
	c := NewFixedChunker(10, 2)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")
	assert.Empty(t, chunks)
}

func TestFixedChunker_ChunkIDs(t *testing.T) {
	reader := elementReaderFromText("docs/page.md", words(80))
	c := NewFixedChunker(30, 5)
	chunks := collectChunks(t, context.Background(), c, reader, "docs/page.md")

	if len(chunks) > 0 {
		assert.Equal(t, "docs/page.md-chunk-0000", chunks[0].ID)
	}
	if len(chunks) > 1 {
		assert.Equal(t, "docs/page.md-chunk-0001", chunks[1].ID)
	}
}

func TestFixedChunker_DocumentPath(t *testing.T) {
	reader := elementReaderFromText("foo/bar.md", words(20))
	c := NewFixedChunker(10, 0)
	chunks := collectChunks(t, context.Background(), c, reader, "foo/bar.md")

	for _, ch := range chunks {
		assert.Equal(t, "foo/bar.md", ch.DocumentPath)
	}
}

func TestFixedChunker_TokenCount(t *testing.T) {
	reader := elementReaderFromText("doc.md", words(40))
	c := NewFixedChunker(10, 0)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")

	for _, ch := range chunks {
		expected := len(ch.Content) / 4
		assert.Equal(t, expected, ch.TokenCount)
	}
}

func TestFixedChunker_MetadataNil(t *testing.T) {
	reader := elementReaderFromText("doc.md", words(5))
	c := NewFixedChunker(10, 0)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")

	for _, ch := range chunks {
		assert.Nil(t, ch.Metadata)
	}
}

func TestFixedChunker_MultipleElements(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementParagraph, Text: "A B C D E"},
		{Kind: types.ElementParagraph, Text: "F G H I J"},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewFixedChunker(5, 0)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 2)
	assert.Equal(t, "A B C D E", chunks[0].Content)
	assert.Equal(t, "F G H I J", chunks[1].Content)
}

func TestFixedChunker_ElementBoundaryCrossing(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementParagraph, Text: "A B C"},
		{Kind: types.ElementParagraph, Text: "D E F G H I"},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewFixedChunker(5, 2)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")

	assert.GreaterOrEqual(t, len(chunks), 2)
	assert.Equal(t, "A B C D E", chunks[0].Content, "first chunk should cross element boundary")
	assert.Equal(t, "D E F G H", chunks[1].Content, "second chunk should start with overlap")
}

func TestFixedChunker_ContextCancellation(t *testing.T) {
	reader := elementReaderFromText("doc.md", words(1000))
	c := NewFixedChunker(30, 10)

	ctx, cancel := context.WithCancel(context.Background())
	chunkCh, errCh := c.Chunk(ctx, reader, "doc.md")

	_, ok := <-chunkCh
	require.True(t, ok)

	cancel()

	for range chunkCh {
	}

	err := <-errCh
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFixedChunker_ExactMatchWithOldImpl(t *testing.T) {
	docContent := words(107)
	docPath := "test/doc.md"

	// Old batch implementation (kept as reference oracle)
	oldChunk := func(content string) ([]types.Chunk, error) {
		words := strings.Fields(content)
		if len(words) == 0 {
			return nil, nil
		}
		step := 30 - 10
		if step <= 0 {
			step = 1
		}
		var chunks []types.Chunk
		idx := 0
		for start := 0; start < len(words); start += step {
			end := start + 30
			if end > len(words) {
				end = len(words)
			}
			chunkWords := words[start:end]
			chunkContent := strings.Join(chunkWords, " ")
			chunks = append(chunks, types.Chunk{
				ID:           fmt.Sprintf("%s-chunk-%04d", docPath, idx),
				DocumentPath: docPath,
				Content:      chunkContent,
				TokenCount:   len(chunkContent) / 4,
				Index:        idx,
			})
			idx++
			if end == len(words) {
				break
			}
		}
		return chunks, nil
	}

	oldChunks, err := oldChunk(docContent)
	require.NoError(t, err)

	reader := elementReaderFromText(docPath, docContent)
	c := NewFixedChunker(30, 10)
	newChunks := collectChunks(t, context.Background(), c, reader, docPath)

	require.Equal(t, len(oldChunks), len(newChunks), "chunk count must match")

	for i := range oldChunks {
		assert.Equal(t, oldChunks[i].ID, newChunks[i].ID, "chunk %d ID", i)
		assert.Equal(t, oldChunks[i].Content, newChunks[i].Content, "chunk %d content", i)
		assert.Equal(t, oldChunks[i].TokenCount, newChunks[i].TokenCount, "chunk %d TokenCount", i)
		assert.Equal(t, oldChunks[i].DocumentPath, newChunks[i].DocumentPath, "chunk %d DocumentPath", i)
	}
}

func TestFixedChunker_ExactMultiple(t *testing.T) {
	reader := elementReaderFromText("doc.md", words(100))
	c := NewFixedChunker(50, 0)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 2)
	wc1 := len(strings.Fields(chunks[0].Content))
	wc2 := len(strings.Fields(chunks[1].Content))
	assert.Equal(t, 50, wc1)
	assert.Equal(t, 50, wc2)
}

type metadataElementReader struct {
	sliceElementReader
	meta map[string]string
}

func (r *metadataElementReader) Metadata() map[string]string {
	return r.meta
}

func TestFixedChunker_MetadataFromReader(t *testing.T) {
	reader := &metadataElementReader{
		sliceElementReader: sliceElementReader{
			elems: []types.Element{
				{Kind: types.ElementParagraph, Text: words(10)},
			},
			path: "handbook/travel-policy.md",
		},
		meta: map[string]string{
			"source_url": "https://handbook.gitlab.com/handbook/travel-policy/",
			"title":      "Travel Policy",
		},
	}

	c := NewFixedChunker(50, 0)
	chunks := collectChunks(t, context.Background(), c, reader, "handbook/travel-policy.md")

	require.NotEmpty(t, chunks)
	for _, ch := range chunks {
		assert.Equal(t, "https://handbook.gitlab.com/handbook/travel-policy/", ch.Metadata["source_url"])
		assert.Equal(t, "Travel Policy", ch.Metadata["title"])
	}
}

func TestFixedChunker_NilMetadata(t *testing.T) {
	reader := elementReaderFromText("doc.md", words(10))
	c := NewFixedChunker(50, 0)
	chunks := collectChunks(t, context.Background(), c, reader, "doc.md")

	require.NotEmpty(t, chunks)
	for _, ch := range chunks {
		assert.Nil(t, ch.Metadata)
	}
}
