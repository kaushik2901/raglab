package preprocessor

import (
	"context"
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

	doc, err := ProcessFile(filePath, dir, "")
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

	doc, err := ProcessFile(filePath, dir, "")
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

	doc, err := ProcessFile(filePath, dir, "")
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

	doc, err := ProcessFile(filePath, dir, "")
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

	doc, err := ProcessFile(filePath, dir, "")
	require.NoError(t, err)
	assert.Greater(t, doc.Size, int64(0))
}

func TestProcessFile_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ProcessFile(filepath.Join(dir, "nonexistent.md"), dir, "")
	require.Error(t, err)
}

func TestProcessFile_RelativePath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	filePath := filepath.Join(subDir, "nested.md")
	os.WriteFile(filePath, []byte("# Nested"), 0644)

	doc, err := ProcessFile(filePath, dir, "")
	require.NoError(t, err)
	assert.Equal(t, "sub/nested.md", doc.Path)
}

func TestProcessAllFiles_SingleFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "page.md"), []byte("# Hello"), 0644)

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 1, "")
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

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 2, "")
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

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 2, "")
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

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 1, "")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	assert.FileExists(t, filepath.Join(dstDir, "a.md"))
	assert.NoFileExists(t, filepath.Join(dstDir, "data.json"))
}

func TestProcessAllFiles_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 1, "")
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

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 5, "")
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

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 0, "")
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

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 1, "")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestProcessFile_WithBaseURL_InjectsSourceURL(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		baseURL  string
		expected string
	}{
		{
			name:     "simple page",
			path:     "handbook/travel-policy.md",
			baseURL:  "https://handbook.gitlab.com",
			expected: "https://handbook.gitlab.com/handbook/travel-policy/",
		},
		{
			name:     "index file",
			path:     "handbook/_index.md",
			baseURL:  "https://handbook.gitlab.com",
			expected: "https://handbook.gitlab.com/handbook/",
		},
		{
			name:     "nested index",
			path:     "handbook/travel-policy/index.md",
			baseURL:  "https://handbook.gitlab.com",
			expected: "https://handbook.gitlab.com/handbook/travel-policy/",
		},
		{
			name:     "top-level page",
			path:     "about.md",
			baseURL:  "https://handbook.gitlab.com",
			expected: "https://handbook.gitlab.com/about/",
		},
		{
			name:     "base URL with trailing slash",
			path:     "faq.md",
			baseURL:  "https://handbook.gitlab.com/",
			expected: "https://handbook.gitlab.com/faq/",
		},
		{
			name:     "empty base URL",
			path:     "page.md",
			baseURL:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			filePath := filepath.Join(dir, filepath.FromSlash(tt.path))
			os.MkdirAll(filepath.Dir(filePath), 0755)
			os.WriteFile(filePath, []byte("# Content"), 0644)

			doc, err := ProcessFile(filePath, dir, tt.baseURL)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, doc.SourceURL)

			if tt.baseURL != "" {
				assert.True(t, HasFrontMatter(doc.Content), "expected content to have front matter with source_url")
				assert.Contains(t, doc.Content, tt.expected)
			}
		})
	}
}

func TestProcessFile_WithBaseURL_PreservesExistingFrontMatter(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	content := "---\ntitle: Original\ndate: 2024-01-01\n---\n# Body"
	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir, "https://example.com")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/doc/", doc.SourceURL)
	assert.Contains(t, doc.Content, "title: Original")
	assert.Contains(t, doc.Content, "date: 2024-01-01")
	assert.Contains(t, doc.Content, "source_url: https://example.com/doc/")
	assert.Contains(t, doc.Content, "# Body")
}

func TestProcessFile_WithBaseURL_OverwritesExistingSourceURL(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	content := "---\nsource_url: https://old.com/\n---\nBody"
	os.WriteFile(filePath, []byte(content), 0644)

	doc, err := ProcessFile(filePath, dir, "https://new.com")
	require.NoError(t, err)
	assert.Equal(t, "https://new.com/doc/", doc.SourceURL)
	assert.Contains(t, doc.Content, "source_url: https://new.com/doc/")
	assert.NotContains(t, doc.Content, "https://old.com/")
}

func TestProcessFile_WithBaseURL_NoFrontMatterCreatesBlock(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "plain.md")
	os.WriteFile(filePath, []byte("# Just heading"), 0644)

	doc, err := ProcessFile(filePath, dir, "https://example.com")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/plain/", doc.SourceURL)
	assert.True(t, HasFrontMatter(doc.Content))
	assert.Contains(t, doc.Content, "source_url: https://example.com/plain/")
	assert.Contains(t, doc.Content, "# Just heading")
}

func TestProcessAllFiles_WithBaseURL_AllFilesGetSourceURL(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.MkdirAll(filepath.Join(srcDir, "handbook"), 0755)
	os.WriteFile(filepath.Join(srcDir, "index.md"), []byte("# Home"), 0644)
	os.WriteFile(filepath.Join(srcDir, "handbook", "policy.md"), []byte("# Policy"), 0644)

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 2, "https://handbook.gitlab.com")
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	homeContent, err := os.ReadFile(filepath.Join(dstDir, "index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(homeContent), "source_url: https://handbook.gitlab.com/index/")

	policyContent, err := os.ReadFile(filepath.Join(dstDir, "handbook", "policy.md"))
	require.NoError(t, err)
	assert.Contains(t, string(policyContent), "source_url: https://handbook.gitlab.com/handbook/policy/")
}

func TestProcessAllFiles_WithoutBaseURL_NoFrontMatter(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "page.md"), []byte("# No URL"), 0644)

	count, err := ProcessAllFiles(context.Background(), srcDir, nil, dstDir, 1, "")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	content, err := os.ReadFile(filepath.Join(dstDir, "page.md"))
	require.NoError(t, err)
	assert.False(t, HasFrontMatter(string(content)))
}
