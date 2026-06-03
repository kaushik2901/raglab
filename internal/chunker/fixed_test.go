package chunker

import (
	"strings"
	"testing"

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
	if err != nil {
		t.Fatal(err)
	}
	step := 20
	expected := (100 + step - 1) / step
	if len(chunks) != expected {
		t.Errorf("got %d chunks, want %d", len(chunks), expected)
	}
	totalWords := 0
	for _, ch := range chunks {
		totalWords += len(strings.Fields(ch.Content))
	}
	if totalWords < 100 {
		t.Errorf("total words %d < 100", totalWords)
	}
}

func TestFixedChunker_NoOverlap(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(100)}
	c := NewFixedChunker(30, 0)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range chunks {
		wc := len(strings.Fields(ch.Content))
		if wc > 30 {
			t.Errorf("chunk has %d words, want <= 30", wc)
		}
	}
}

func TestFixedChunker_FullOverlap(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(50)}
	c := NewFixedChunker(10, 20)
	if c.Overlap != 9 {
		t.Errorf("Overlap clamped to %d, want 9", c.Overlap)
	}
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 1 {
		t.Fatal("expected at least 1 chunk")
	}
}

func TestFixedChunker_EmptyDoc(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: ""}
	c := NewFixedChunker(10, 2)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Errorf("got %d chunks, want 0", len(chunks))
	}
}

func TestFixedChunker_ShortDoc(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(3)}
	c := NewFixedChunker(100, 10)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Errorf("got %d chunks, want 1", len(chunks))
	}
}

func TestFixedChunker_ExactMultiple(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(100)}
	c := NewFixedChunker(50, 0)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Errorf("got %d chunks, want 2", len(chunks))
	}
	wc1 := len(strings.Fields(chunks[0].Content))
	wc2 := len(strings.Fields(chunks[1].Content))
	if wc1 != 50 || wc2 != 50 {
		t.Errorf("chunk sizes: %d, %d, want both 50", wc1, wc2)
	}
}

func TestFixedChunker_SingleWord(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: "hello"}
	c := NewFixedChunker(10, 2)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Errorf("got %d chunks, want 1", len(chunks))
	}
}

func TestFixedChunker_WhitespaceOnly(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: "   \n  \t  "}
	c := NewFixedChunker(10, 2)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Errorf("got %d chunks, want 0", len(chunks))
	}
}

func TestFixedChunker_ChunkIDs(t *testing.T) {
	doc := types.Document{Path: "docs/page.md", Content: words(80)}
	c := NewFixedChunker(30, 5)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	for i, ch := range chunks {
		want := "docs/page.md-chunk-0000"
		if i > 0 {
			want = "docs/page.md-chunk-0001"
		}
		if i >= 2 {
			break
		}
		if ch.ID != want {
			t.Errorf("chunk[%d].ID = %q, want %q", i, ch.ID, want)
		}
	}
}

func TestFixedChunker_DocumentPath(t *testing.T) {
	doc := types.Document{Path: "foo/bar.md", Content: words(20)}
	c := NewFixedChunker(10, 0)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range chunks {
		if ch.DocumentPath != "foo/bar.md" {
			t.Errorf("DocumentPath = %q, want %q", ch.DocumentPath, "foo/bar.md")
		}
	}
}

func TestFixedChunker_TokenCount(t *testing.T) {
	content := words(40)
	doc := types.Document{Path: "doc.md", Content: content}
	c := NewFixedChunker(10, 0)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range chunks {
		expected := len(ch.Content) / 4
		if ch.TokenCount != expected {
			t.Errorf("TokenCount = %d, want %d (content len %d)", ch.TokenCount, expected, len(ch.Content))
		}
	}
}

func TestFixedChunker_MetadataNil(t *testing.T) {
	doc := types.Document{Path: "doc.md", Content: words(5)}
	c := NewFixedChunker(10, 0)
	chunks, err := c.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range chunks {
		if ch.Metadata != nil {
			t.Error("Metadata should be nil for plain text")
		}
	}
}
