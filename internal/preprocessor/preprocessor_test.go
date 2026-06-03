package preprocessor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessFile_SimpleMarkdown(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	content := "# Hello World\n\nThis is **bold** text."
	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	require.NoError(t, err)
	assert.Equal(t, "test.md", doc.Path)
	assert.NotEmpty(t, doc.Content)
	assert.Greater(t, doc.Size, int64(0))
}

func TestProcessFile_WithIncludes(t *testing.T) {
	dir := t.TempDir()
	snippetPath := filepath.Join(dir, "snippet.md")
	os.WriteFile(snippetPath, []byte("included text"), 0644)

	filePath := filepath.Join(dir, "main.md")
	content := `# Main

Before {{% include "snippet.md" %}} After`

	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	require.NoError(t, err)
	assert.Equal(t, "main.md", doc.Path)
}

func TestProcessFile_WithHTML(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "page.md")
	content := `# Page

<style>body{color:red}</style>
<div class="main">Content</div>
<a href="https://example.com">Link</a>`

	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	require.NoError(t, err)
	assert.Greater(t, doc.Size, int64(0))
}

func TestProcessFile_WithShortcodes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	content := `# Doc

{{< details >}}secret info{{< /details >}}
{{% alert %}}warning{{% /alert %}}`

	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	require.NoError(t, err)
	assert.Greater(t, doc.Size, int64(0))
}

func TestProcessFile_WithRefs(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "other.md")
	os.WriteFile(targetPath, []byte("other content"), 0644)

	filePath := filepath.Join(dir, "page.md")
	content := `See {{< ref "other" >}} for details`
	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir)
	require.NoError(t, err)
	assert.Greater(t, doc.Size, int64(0))
}

func TestProcessFile_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ProcessFile(filepath.Join(dir, "nonexistent.md"), dir)
	require.Error(t, err)
}

func TestProcessFile_RelativePath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	filePath := filepath.Join(subDir, "nested.md")
	os.WriteFile(filePath, []byte("# Nested"), 0644)

	doc, err := ProcessFile(filePath, dir)
	require.NoError(t, err)
	assert.Equal(t, "sub/nested.md", doc.Path)
}

func TestProcessAllFiles_SingleFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "page.md"), []byte("# Hello"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.FileExists(t, filepath.Join(dstDir, "page.md"))
}

func TestProcessAllFiles_MultipleFiles(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("A"), 0644)
	os.WriteFile(filepath.Join(srcDir, "b.md"), []byte("B"), 0644)
	os.WriteFile(filepath.Join(srcDir, "c.md"), []byte("C"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	for _, name := range []string{"a.md", "b.md", "c.md"} {
		assert.FileExists(t, filepath.Join(dstDir, name))
	}
}

func TestProcessAllFiles_PreservesDirectoryStructure(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "index.md"), []byte("# Index"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "page.md"), []byte("# Sub Page"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	assert.FileExists(t, filepath.Join(dstDir, "index.md"))
	assert.FileExists(t, filepath.Join(dstDir, "sub", "page.md"))
}

func TestProcessAllFiles_SkipsNonMarkdown(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("md"), 0644)
	os.WriteFile(filepath.Join(srcDir, "data.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(srcDir, "notes.txt"), []byte("text"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	assert.FileExists(t, filepath.Join(dstDir, "a.md"))
	assert.NoFileExists(t, filepath.Join(dstDir, "data.json"))
}

func TestProcessAllFiles_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	count, err := ProcessAllFiles(srcDir, dstDir, 1)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestProcessAllFiles_WithConcurrency(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("file%d.md", i)
		os.WriteFile(filepath.Join(srcDir, name), []byte("# File "+name), 0644)
	}

	count, err := ProcessAllFiles(srcDir, dstDir, 5)
	require.NoError(t, err)
	assert.Equal(t, 10, count)

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("file%d.md", i)
		assert.FileExists(t, filepath.Join(dstDir, name))
	}
}

func TestProcessAllFiles_DefaultConcurrency(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "x.md"), []byte("x"), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestProcessAllFiles_ContentTransformed(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	snippetPath := filepath.Join(srcDir, "snippet.md")
	os.WriteFile(snippetPath, []byte("snippet content"), 0644)

	filePath := filepath.Join(srcDir, "main.md")
	content := `# Main

<style>.bold{color:red}</style>
{{< details >}}secret{{< /details >}}
{{% include "snippet.md" %}}
See {{< ref "snippet" >}}`

	os.WriteFile(filePath, []byte(content), 0644)

	count, err := ProcessAllFiles(srcDir, dstDir, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
