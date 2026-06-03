package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDir_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "c.md"), []byte("# C"), 0644)

	docs, err := ParseDir(dir)
	require.NoError(t, err)
	assert.Len(t, docs, 3)

	paths := make(map[string]string)
	for _, d := range docs {
		paths[d.Path] = d.Content
	}
	assert.Equal(t, "# A", paths["a.md"])
	assert.Equal(t, "# B", paths["b.md"])
	assert.Equal(t, "# C", paths["sub/c.md"])
}

func TestParseDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	docs, err := ParseDir(dir)
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestParseDir_NoMdFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte("{}"), 0644)

	docs, err := ParseDir(dir)
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestParseDir_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "visible.md"), []byte("visible"), 0644)
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)
	os.WriteFile(filepath.Join(dir, ".hidden", "secret.md"), []byte("secret"), 0644)

	docs, err := ParseDir(dir)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

func TestParseDir_SkipsNonMd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("text"), 0644)
	os.WriteFile(filepath.Join(dir, "c.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "d"), []byte("noext"), 0644)

	docs, err := ParseDir(dir)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

func TestParseDir_Subdirectories(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.md"), []byte("root"), 0644)
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "x.md"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "y.md"), []byte("y"), 0644)

	docs, err := ParseDir(dir)
	require.NoError(t, err)
	assert.Len(t, docs, 3)
}

func TestParseDir_RelativePaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "doc.md"), []byte("hello"), 0644)

	docs, err := ParseDir(dir)
	require.NoError(t, err)
	require.Len(t, docs, 1)

	assert.Equal(t, "sub/doc.md", docs[0].Path)
	assert.False(t, strings.HasPrefix(docs[0].Path, "./"))
}

func TestParseDir_EmptyFileContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.md"), []byte{}, 0644)

	docs, err := ParseDir(dir)
	require.NoError(t, err)
	require.Len(t, docs, 1)

	assert.Equal(t, "", docs[0].Content)
	assert.Equal(t, int64(0), docs[0].Size)
}

func TestParseDir_LargeFile(t *testing.T) {
	dir := t.TempDir()
	content := make([]byte, 2*1024*1024)
	for i := range content {
		content[i] = 'a'
	}
	os.WriteFile(filepath.Join(dir, "large.md"), content, 0644)

	docs, err := ParseDir(dir)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Len(t, docs[0].Content, 2*1024*1024)
}

func TestParseDir_NonExistentDir(t *testing.T) {
	_, err := ParseDir("C:\\nonexistent-path-that-does-not-exist-12345")
	assert.Error(t, err)
}

func TestParseDir_PermissionDenied(t *testing.T) {
	dir := t.TempDir()
	restricted := filepath.Join(dir, "restricted")
	os.MkdirAll(restricted, 0755)
	os.WriteFile(filepath.Join(dir, "ok.md"), []byte("ok"), 0644)
	os.WriteFile(filepath.Join(restricted, "secret.md"), []byte("secret"), 0644)

	if err := os.Chmod(restricted, 0000); err != nil {
		t.Skipf("cannot chmod on this platform: %v", err)
	}
	defer os.Chmod(restricted, 0755)

	_, err := ParseDir(dir)
	if err == nil {
		t.Skip("chmod 0000 did not cause error on this platform (e.g., Windows)")
	}
}

func TestParseFile_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("# Hello\nWorld"), 0644)

	doc, err := ParseFile(path, "test.md")
	require.NoError(t, err)

	assert.Equal(t, "test.md", doc.Path)
	assert.Equal(t, "# Hello\nWorld", doc.Content)
	assert.Equal(t, int64(13), doc.Size)
}

func TestParseFile_Error(t *testing.T) {
	_, err := ParseFile("C:\\nonexistent-file-12345.md", "nope.md")
	assert.Error(t, err)
}
