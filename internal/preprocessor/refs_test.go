package preprocessor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRefs_NoRefs(t *testing.T) {
	content := "# Hello\n\nThis is plain text."
	result, err := ResolveRefs(content, "/tmp", "/tmp/file.md")
	require.NoError(t, err)
	assert.Equal(t, content, result)
}

func TestResolveRefs_Ref(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "docs", "foo.md")
	os.MkdirAll(filepath.Dir(targetPath), 0755)
	os.WriteFile(targetPath, []byte("content"), 0644)

	content := `See {{< ref "docs/foo" >}} for details`
	currentFile := filepath.Join(dir, "current.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	result, err := ResolveRefs(content, dir, currentFile)
	require.NoError(t, err)
	assert.Equal(t, "See [docs/foo](docs/foo.md) for details", result)
}

func TestResolveRefs_Relref(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	targetPath := filepath.Join(subDir, "bar.md")
	os.WriteFile(targetPath, []byte("content"), 0644)

	currentFile := filepath.Join(subDir, "page.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `See {{< relref "bar" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	require.NoError(t, err)
	assert.Equal(t, "See [bar](bar.md)", result)
}

func TestResolveRefs_WithAnchor(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "docs", "page.md")
	os.MkdirAll(filepath.Dir(targetPath), 0755)
	os.WriteFile(targetPath, []byte("content"), 0644)

	currentFile := filepath.Join(dir, "current.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `See {{< ref "docs/page#section1" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	require.NoError(t, err)
	assert.Equal(t, "See [docs/page#section1](docs/page.md#section1)", result)
}

func TestResolveRefs_MissingTarget(t *testing.T) {
	dir := t.TempDir()
	currentFile := filepath.Join(dir, "current.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `See {{< ref "nonexistent" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	require.Error(t, err)
	assert.Equal(t, "See [nonexistent](nonexistent.md)", result)
}

func TestResolveRefs_MultipleRefs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte(""), 0644)

	currentFile := filepath.Join(dir, "current.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `See {{< ref "a" >}} and {{< ref "b" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	require.NoError(t, err)
	assert.Equal(t, "See [a](a.md) and [b](b.md)", result)
}

func TestResolveRefs_MixedRefTypes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte(""), 0644)
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "other.md"), []byte(""), 0644)

	currentFile := filepath.Join(subDir, "page.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `Global {{< ref "doc" >}} and local {{< relref "other" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	require.NoError(t, err)
	assert.Equal(t, "Global [doc](doc.md) and local [other](other.md)", result)
}

func TestResolveRefs_RefWithMD(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "foo.md"), []byte(""), 0644)

	currentFile := filepath.Join(dir, "c.md")
	os.WriteFile(currentFile, []byte(""), 0644)

	content := `{{< ref "foo" >}}`
	result, err := ResolveRefs(content, dir, currentFile)
	require.NoError(t, err)
	assert.Equal(t, "[foo](foo.md)", result)
}

func TestResolveRefs_EmptyContent(t *testing.T) {
	result, err := ResolveRefs("", "/tmp", "/tmp/f.md")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}
