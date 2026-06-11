package chunker

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func collectRecursiveChunks(t *testing.T, ctx context.Context, c *RecursiveChunker, reader types.ElementReader, docPath string) []types.Chunk {
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

func TestRecursiveChunker_BasicSection(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Travel Policy"},
		{Kind: types.ElementParagraph, Text: "Employees must use the corporate portal."},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "Travel Policy")
	assert.Contains(t, chunks[0].Content, "Employees must use the corporate portal")
}

func TestRecursiveChunker_MultipleH1Sections(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Section One"},
		{Kind: types.ElementParagraph, Text: "Content of section one."},
		{Kind: types.ElementHeading, Level: 1, Text: "Section Two"},
		{Kind: types.ElementParagraph, Text: "Content of section two."},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 2)
	assert.Contains(t, chunks[0].Content, "Section One")
	assert.Contains(t, chunks[1].Content, "Section Two")
}

func TestRecursiveChunker_NestedHierarchy(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Travel"},
		{Kind: types.ElementHeading, Level: 2, Text: "Booking Flights"},
		{Kind: types.ElementParagraph, Text: "Book through Concur."},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "Travel > Booking Flights")
	assert.Contains(t, chunks[0].Content, "Book through Concur.")
}

func TestRecursiveChunker_DeepNesting(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "A"},
		{Kind: types.ElementHeading, Level: 2, Text: "B"},
		{Kind: types.ElementHeading, Level: 3, Text: "C"},
		{Kind: types.ElementParagraph, Text: "Deep content here."},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "A > B > C")
	assert.Contains(t, chunks[0].Content, "Deep content here.")
}

func TestRecursiveChunker_NoHeadings(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementParagraph, Text: "Just a plain paragraph."},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1)
	assert.Equal(t, "Just a plain paragraph.", chunks[0].Content)
}

func TestRecursiveChunker_HeadingLevelJump(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Top"},
		{Kind: types.ElementHeading, Level: 3, Text: "Jump"},
		{Kind: types.ElementParagraph, Text: "Jumped content."},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "Top > Jump")
	assert.Contains(t, chunks[0].Content, "Jumped content.")
}

func TestRecursiveChunker_SplitAtH2(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Guide"},
		{Kind: types.ElementHeading, Level: 2, Text: "Install"},
		{Kind: types.ElementParagraph, Text: words(200)},
		{Kind: types.ElementHeading, Level: 2, Text: "Config"},
		{Kind: types.ElementParagraph, Text: words(200)},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(250, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 2)
	assert.Contains(t, chunks[0].Content, "Guide > Install")
	assert.Contains(t, chunks[1].Content, "Guide > Config")
}

func TestRecursiveChunker_SplitAtH2WithWordFallback(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Guide"},
		{Kind: types.ElementHeading, Level: 2, Text: "Install"},
		{Kind: types.ElementParagraph, Text: words(200)},
		{Kind: types.ElementHeading, Level: 2, Text: "Config"},
		{Kind: types.ElementParagraph, Text: words(50)},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 10)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	// "Install" (200 words) > MaxSize=100 → falls through to word split → multiple chunks
	// "Config" (50 words) ≤ MaxSize=100 → one chunk
	require.Greater(t, len(chunks), 2)
	assert.Contains(t, chunks[0].Content, "Guide > Install")
	assert.Contains(t, chunks[len(chunks)-1].Content, "Guide > Config")
}

func TestRecursiveChunker_LargeSectionFallback(t *testing.T) {
	text := strings.Join(strings.Fields(words(200)), " ")
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Big Section"},
		{Kind: types.ElementParagraph, Text: text},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(50, 10)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Greater(t, len(chunks), 1, "large section should be split")
	for _, ch := range chunks {
		assert.Contains(t, ch.Content, "Big Section", "all chunks should have heading path")
	}
}

func TestRecursiveChunker_PreHeadingContent(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementParagraph, Text: "Preamble text."},
		{Kind: types.ElementHeading, Level: 1, Text: "Main"},
		{Kind: types.ElementParagraph, Text: "Main text."},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 2)
	assert.Contains(t, chunks[0].Content, "Preamble text.")
	assert.Contains(t, chunks[1].Content, "Main")
	assert.Contains(t, chunks[1].Content, "Main text.")
}

func TestRecursiveChunker_AtomicCodeBlock(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Code"},
		{Kind: types.ElementCodeBlock, Text: "def hello():\n    print('world')"},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "def hello()")
	assert.Contains(t, chunks[0].Content, "print('world')")
}

func TestRecursiveChunker_AtomicTable(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Table"},
		{Kind: types.ElementTable, Text: "| A | B |\n| 1 | 2 |"},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "| A | B |")
}

func TestRecursiveChunker_EmptyDocument(t *testing.T) {
	reader := elementReaderFromElements("doc.md")
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")
	assert.Empty(t, chunks)
}

func TestRecursiveChunker_AllElementTypes(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Mixed"},
		{Kind: types.ElementParagraph, Text: "Para text."},
		{Kind: types.ElementCodeBlock, Text: "code block"},
		{Kind: types.ElementTable, Text: "table data"},
		{Kind: types.ElementListItem, Text: "list item"},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1)
	for _, s := range []string{"Para text.", "code block", "table data", "list item"} {
		assert.Contains(t, chunks[0].Content, s)
	}
}

func TestRecursiveChunker_ContextCancellation(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Section"},
		{Kind: types.ElementParagraph, Text: words(1000)},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(50, 0)

	ctx, cancel := context.WithCancel(context.Background())
	chunkCh, errCh := c.Chunk(ctx, reader, "doc.md")

	_, ok := <-chunkCh
	require.True(t, ok)

	cancel()

	for range chunkCh {
	}

	err := <-errCh
	assert.Error(t, err)
}

func TestRecursiveChunker_MetadataPropagation(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Section"},
		{Kind: types.ElementParagraph, Text: "Some content."},
	}
	reader := &metadataElementReader{
		sliceElementReader: sliceElementReader{
			elems: elems,
			path:  "doc.md",
		},
		meta: map[string]string{"title": "Test Doc"},
	}
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.NotEmpty(t, chunks)
	for _, ch := range chunks {
		assert.Equal(t, "Test Doc", ch.Metadata["title"])
	}
}

func TestRecursiveChunker_ChunkIDs(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementParagraph, Text: "One."},
		{Kind: types.ElementParagraph, Text: "Two."},
	}
	reader := elementReaderFromElements("handbook/page.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "handbook/page.md")

	require.Len(t, chunks, 1)
	assert.Equal(t, "handbook/page.md-chunk-0000", chunks[0].ID)
}

func TestRecursiveChunker_DocumentPath(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementParagraph, Text: "Content."},
	}
	reader := elementReaderFromElements("foo/bar.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "foo/bar.md")

	require.NotEmpty(t, chunks)
	for _, ch := range chunks {
		assert.Equal(t, "foo/bar.md", ch.DocumentPath)
	}
}

func TestRecursiveChunker_TokenCount(t *testing.T) {
	elems := []types.Element{
		{Kind: types.ElementParagraph, Text: "Hello world test content."},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(100, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.NotEmpty(t, chunks)
	expected := len(chunks[0].Content) / 4
	assert.Equal(t, expected, chunks[0].TokenCount)
}

func TestRecursiveChunker_OversizedCodeBlockStaysIntact(t *testing.T) {
	codeText := "def hello():\n    print('world')\n    return 42\n"
	for i := 0; i < 50; i++ {
		codeText += "    print('line " + strings.Repeat("x", 20) + "')\n"
	}
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Code"},
		{Kind: types.ElementCodeBlock, Text: codeText},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(30, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1, "oversized code block should be a single chunk")
	assert.Contains(t, chunks[0].Content, "def hello()")
}

func TestRecursiveChunker_OversizedTableStaysIntact(t *testing.T) {
	tableText := "| A | B |\n"
	for i := 0; i < 100; i++ {
		tableText += "| " + strings.Repeat("x", 20) + " | " + strings.Repeat("y", 20) + " |\n"
	}
	elems := []types.Element{
		{Kind: types.ElementHeading, Level: 1, Text: "Table"},
		{Kind: types.ElementTable, Text: tableText},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(30, 0)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Len(t, chunks, 1, "oversized table should be a single chunk")
	assert.Contains(t, chunks[0].Content, "| A | B |")
}

func TestRecursiveChunker_HardWordSplitOverlap(t *testing.T) {
	text := strings.Join(strings.Fields(words(100)), " ")
	elems := []types.Element{
		{Kind: types.ElementParagraph, Text: text},
	}
	reader := elementReaderFromElements("doc.md", elems...)
	c := NewRecursiveChunker(30, 5)
	chunks := collectRecursiveChunks(t, context.Background(), c, reader, "doc.md")

	require.Greater(t, len(chunks), 1)

	totalWords := 0
	for _, ch := range chunks {
		totalWords += len(strings.Fields(ch.Content))
	}
	assert.Greater(t, totalWords, 100, "overlap should produce more total words")
}
