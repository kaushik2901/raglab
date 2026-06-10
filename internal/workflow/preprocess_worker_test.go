package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/riverqueue/river"
)

func TestPreprocessArgs_Kind(t *testing.T) {
	assert.Equal(t, "preprocess", PreprocessArgs{}.Kind())
}

func TestReadCheckpoint_Empty(t *testing.T) {
	job := &river.Job[PreprocessArgs]{
		JobRow: &rivertype.JobRow{},
	}
	cp := readCheckpoint(job)
	assert.NotNil(t, cp)
	assert.False(t, cp["clone_done"])
	assert.False(t, cp["preprocess_done"])
}

func TestCollectMarkdownPathsInDirs_EmptyIncludeDirs(t *testing.T) {
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("A"), 0644)
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "sub", "b.md"), []byte("B"), 0644)

	paths, err := collectMarkdownPathsInDirs(srcDir, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, len(paths))
	assert.True(t, paths["a.md"])
	assert.True(t, paths[filepath.FromSlash("sub/b.md")])
}

func TestCollectMarkdownPathsInDirs_WithIncludeDirs(t *testing.T) {
	srcDir := t.TempDir()

	os.MkdirAll(filepath.Join(srcDir, "handbook", "engineering", "ai", "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "handbook", "engineering", "ai", "index.md"), []byte("# AI"), 0644)
	os.WriteFile(filepath.Join(srcDir, "handbook", "engineering", "ai", "sub", "page.md"), []byte("# Sub"), 0644)
	os.WriteFile(filepath.Join(srcDir, "other.md"), []byte("# Other"), 0644)

	paths, err := collectMarkdownPathsInDirs(srcDir, []string{"handbook/engineering/ai"})
	require.NoError(t, err)

	assert.Equal(t, 2, len(paths), "should only include files under the specified include dir")
	assert.True(t, paths[filepath.FromSlash("handbook/engineering/ai/index.md")],
		"path should be relative to srcDir, not the include subdirectory")
	assert.True(t, paths[filepath.FromSlash("handbook/engineering/ai/sub/page.md")],
		"nested paths should retain full prefix relative to srcDir")
	assert.False(t, paths["other.md"], "files outside include dir should not appear")
}

func TestCheckDirectoryStructure_WithoutIncludeDirs(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "page.md"), []byte("# Page"), 0644)
	os.WriteFile(filepath.Join(dstDir, "page.md"), []byte("# Page"), 0644)

	report := &verificationReport{}
	checkDirectoryStructure(report, srcDir, dstDir, nil)

	require.Len(t, report.Checks, 1)
	assert.True(t, report.Checks[0].Passed)
}

func TestCheckDirectoryStructure_WithIncludeDirs(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.MkdirAll(filepath.Join(srcDir, "handbook", "engineering", "ai"), 0755)
	os.WriteFile(filepath.Join(srcDir, "handbook", "engineering", "ai", "index.md"), []byte("# AI"), 0644)

	os.MkdirAll(filepath.Join(dstDir, "handbook", "engineering", "ai"), 0755)
	os.WriteFile(filepath.Join(dstDir, "handbook", "engineering", "ai", "index.md"), []byte("# AI"), 0644)

	report := &verificationReport{}
	checkDirectoryStructure(report, srcDir, dstDir, []string{"handbook/engineering/ai"})

	assert.True(t, report.Checks[0].Passed,
		"directory structure should match when source and dest have same files under include dir")
}

func TestCheckDirectoryStructure_WithIncludeDirs_Mismatch(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.MkdirAll(filepath.Join(srcDir, "handbook", "engineering", "ai"), 0755)
	os.WriteFile(filepath.Join(srcDir, "handbook", "engineering", "ai", "index.md"), []byte("# AI"), 0644)

	report := &verificationReport{}
	checkDirectoryStructure(report, srcDir, dstDir, []string{"handbook/engineering/ai"})

	assert.False(t, report.Checks[0].Passed,
		"should fail when destination is missing the file")
}
