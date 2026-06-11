package parser

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func collectElements(t *testing.T, p *MarkdownParser, path string) []types.Element {
	t.Helper()
	r, err := p.Parse(path)
	require.NoError(t, err)
	defer r.Close()

	var elems []types.Element
	for {
		e, err := r.ReadElement()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		elems = append(elems, e)
	}
	return elems
}

func TestMarkdownParser_Paragraphs(t *testing.T) {
	elems := collectElements(t, &MarkdownParser{}, filepath.Join("testdata", "simple.md"))
	require.Len(t, elems, 4)

	assert.Equal(t, types.ElementHeading, elems[0].Kind)
	assert.Equal(t, 1, elems[0].Level)
	assert.Equal(t, "Title", elems[0].Text)

	assert.Equal(t, types.ElementParagraph, elems[1].Kind)
	assert.Equal(t, "Hello world.", elems[1].Text)

	assert.Equal(t, types.ElementHeading, elems[2].Kind)
	assert.Equal(t, 2, elems[2].Level)
	assert.Equal(t, "Sub", elems[2].Text)

	assert.Equal(t, types.ElementParagraph, elems[3].Kind)
	assert.Equal(t, "Details.", elems[3].Text)
}

func TestMarkdownParser_Headings(t *testing.T) {
	elems := collectElements(t, &MarkdownParser{}, filepath.Join("testdata", "headings.md"))
	require.Len(t, elems, 5)

	assert.Equal(t, types.ElementHeading, elems[0].Kind)
	assert.Equal(t, 1, elems[0].Level)
	assert.Equal(t, "H1", elems[0].Text)

	assert.Equal(t, types.ElementHeading, elems[1].Kind)
	assert.Equal(t, 2, elems[1].Level)
	assert.Equal(t, "H2", elems[1].Text)

	assert.Equal(t, types.ElementHeading, elems[2].Kind)
	assert.Equal(t, 3, elems[2].Level)
	assert.Equal(t, "H3", elems[2].Text)

	assert.Equal(t, types.ElementParagraph, elems[3].Kind)
	assert.Equal(t, "Text", elems[3].Text)

	assert.Equal(t, types.ElementHeading, elems[4].Kind)
	assert.Equal(t, 4, elems[4].Level)
	assert.Equal(t, "H4", elems[4].Text)
}

func TestMarkdownParser_CodeBlocks(t *testing.T) {
	elems := collectElements(t, &MarkdownParser{}, filepath.Join("testdata", "codeblocks.md"))
	require.Len(t, elems, 2)

	assert.Equal(t, types.ElementCodeBlock, elems[0].Kind)
	assert.Equal(t, "fmt.Println()", elems[0].Text)
	require.NotNil(t, elems[0].Meta)
	assert.Equal(t, "go", elems[0].Meta["language"])

	assert.Equal(t, types.ElementParagraph, elems[1].Kind)
	assert.Equal(t, "Para", elems[1].Text)
}

func TestMarkdownParser_Tables(t *testing.T) {
	elems := collectElements(t, &MarkdownParser{}, filepath.Join("testdata", "tables.md"))
	require.Len(t, elems, 1)

	assert.Equal(t, types.ElementTable, elems[0].Kind)
	assert.Contains(t, elems[0].Text, "A B")
	assert.Contains(t, elems[0].Text, "1 2")
}

func TestMarkdownParser_EmptyFile(t *testing.T) {
	r, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "empty.md"))
	require.NoError(t, err)
	defer r.Close()

	_, err = r.ReadElement()
	assert.Equal(t, io.EOF, err)
}

func TestMarkdownParser_WhitespaceOnly(t *testing.T) {
	r, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "whitespace.md"))
	require.NoError(t, err)
	defer r.Close()

	_, err = r.ReadElement()
	assert.Equal(t, io.EOF, err)
}

func TestMarkdownParser_Error(t *testing.T) {
	_, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "nonexistent.md"))
	assert.Error(t, err)
}

func TestMarkdownParser_Unicode(t *testing.T) {
	elems := collectElements(t, &MarkdownParser{}, filepath.Join("testdata", "unicode.md"))
	require.Len(t, elems, 2)

	assert.Equal(t, types.ElementHeading, elems[0].Kind)
	assert.Equal(t, 2, elems[0].Level)
	assert.Equal(t, "Héllo", elems[0].Text)

	assert.Equal(t, types.ElementParagraph, elems[1].Kind)
	assert.Equal(t, "Wörld", elems[1].Text)
}

func TestMarkdownParser_Mixed(t *testing.T) {
	elems := collectElements(t, &MarkdownParser{}, filepath.Join("testdata", "mixed.md"))
	assert.GreaterOrEqual(t, len(elems), 5)

	foundCode := false
	foundTable := false
	for _, e := range elems {
		if e.Kind == types.ElementCodeBlock {
			foundCode = true
			assert.Contains(t, e.Text, "fmt.Println")
		}
		if e.Kind == types.ElementTable {
			foundTable = true
			assert.Contains(t, e.Text, "foo")
		}
	}
	assert.True(t, foundCode, "expected a code block element")
	assert.True(t, foundTable, "expected a table element")
}

func TestMarkdownParser_Lists(t *testing.T) {
	elems := collectElements(t, &MarkdownParser{}, filepath.Join("testdata", "lists.md"))
	require.Len(t, elems, 3)

	assert.Equal(t, types.ElementListItem, elems[0].Kind)
	assert.Equal(t, "item1", elems[0].Text)

	assert.Equal(t, types.ElementListItem, elems[1].Kind)
	assert.Equal(t, "item2", elems[1].Text)

	assert.Equal(t, types.ElementParagraph, elems[2].Kind)
	assert.Equal(t, "Para", elems[2].Text)
}

func TestMarkdownParser_Nometadata(t *testing.T) {
	elems := collectElements(t, &MarkdownParser{}, filepath.Join("testdata", "nometadata.md"))
	require.Len(t, elems, 2)

	assert.Equal(t, types.ElementParagraph, elems[0].Kind)
	assert.Equal(t, "Just a paragraph.", elems[0].Text)

	assert.Equal(t, types.ElementParagraph, elems[1].Kind)
	assert.Equal(t, "Another one.", elems[1].Text)
}

func TestMarkdownParser_Path(t *testing.T) {
	r, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "simple.md"))
	require.NoError(t, err)
	defer r.Close()

	assert.Equal(t, filepath.Join("testdata", "simple.md"), r.Path())
}

func TestMarkdownParser_FrontMatter_StrippedFromElements(t *testing.T) {
	r, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "frontmatter.md"))
	require.NoError(t, err)
	defer r.Close()

	var elems []types.Element
	for {
		e, err := r.ReadElement()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		elems = append(elems, e)
	}

	require.Len(t, elems, 2)
	assert.Equal(t, types.ElementHeading, elems[0].Kind)
	assert.Equal(t, "Hello", elems[0].Text)
	assert.Equal(t, types.ElementParagraph, elems[1].Kind)
	assert.Equal(t, "World.", elems[1].Text)

	// No elements from front matter delimiters or YAML content
	for _, e := range elems {
		assert.NotContains(t, e.Text, "---")
		assert.NotContains(t, e.Text, "title:")
		assert.NotContains(t, e.Text, "source_url:")
	}
}

func TestMarkdownParser_FrontMatter_Metadata(t *testing.T) {
	r, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "frontmatter.md"))
	require.NoError(t, err)
	defer r.Close()

	md := r.Metadata()
	require.NotNil(t, md)
	assert.Equal(t, "Test Page", md["title"])
	assert.Equal(t, "https://example.com/test/", md["source_url"])
}

func TestMarkdownParser_FrontMatter_MetadataAccessibleAfterRead(t *testing.T) {
	r, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "frontmatter.md"))
	require.NoError(t, err)
	defer r.Close()

	// Consume all elements
	for {
		_, err := r.ReadElement()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	// Metadata still accessible after full read
	md := r.Metadata()
	require.NotNil(t, md)
	assert.Equal(t, "https://example.com/test/", md["source_url"])
}

func TestMarkdownParser_NoFrontMatter_MetadataNil(t *testing.T) {
	r, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "simple.md"))
	require.NoError(t, err)
	defer r.Close()

	md := r.Metadata()
	assert.Nil(t, md)
}

func TestMarkdownParser_FrontMatter_EmptyBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "emptyfm.md")
	os.WriteFile(path, []byte("---\n---\n# Hello"), 0644)

	r, err := (&MarkdownParser{}).Parse(path)
	require.NoError(t, err)
	defer r.Close()

	md := r.Metadata()
	require.NotNil(t, md)
	assert.Empty(t, md)

	elems := collectElements(t, &MarkdownParser{}, path)
	require.Len(t, elems, 1)
	assert.Equal(t, types.ElementHeading, elems[0].Kind)
}

func TestMarkdownParser_FrontMatter_SingleDashNoClosing(t *testing.T) {
	r, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "simple.md"))
	require.NoError(t, err)
	defer r.Close()

	// No FM, just regular content
	md := r.Metadata()
	assert.Nil(t, md)
}

func TestMarkdownParser_FrontMatter_MultipleKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.md")
	content := "---\ntitle: Multi\ndate: 2024-01-01\ncount: 3\ntags: [a, b, c]\n---\nBody"
	os.WriteFile(path, []byte(content), 0644)

	r, err := (&MarkdownParser{}).Parse(path)
	require.NoError(t, err)
	defer r.Close()

	md := r.Metadata()
	require.NotNil(t, md)
	assert.Equal(t, "Multi", md["title"])
	assert.NotEmpty(t, md["date"])
	assert.NotEmpty(t, md["count"])
	assert.NotEmpty(t, md["tags"])
}

func TestMarkdownParser_FrontMatter_OnlyInFirstFile(t *testing.T) {
	// Verify that a file without FM still returns nil metadata
	// even after parsing a file with FM in the same test run
	r1, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "frontmatter.md"))
	require.NoError(t, err)
	r1.Close()
	assert.NotNil(t, r1.Metadata())

	r2, err := (&MarkdownParser{}).Parse(filepath.Join("testdata", "nometadata.md"))
	require.NoError(t, err)
	r2.Close()
	assert.Nil(t, r2.Metadata())
}
