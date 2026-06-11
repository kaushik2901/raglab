package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestIsTransientGitError_TransientPatterns(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
	}{
		{"connection refused", "ssh: connect to host gitlab.com port 22: Connection refused"},
		{"connection reset", "fatal: The remote end hung up unexpectedly\nfatal: early EOF\nfatal: index-pack failed"},
		{"could not resolve host", "fatal: unable to access 'https://gitlab.com/repo.git': Could not resolve host: gitlab.com"},
		{"operation timed out", "fatal: unable to access 'https://gitlab.com/repo.git': Operation timed out after 30000 milliseconds"},
		{"rate limit", "remote: You have exceeded a rate limit. Please wait and try again."},
		{"HTTP 429", "remote: 429 Too Many Requests"},
		{"HTTP 503", "fatal: unable to access 'https://gitlab.com/repo.git': The requested URL returned error: 503"},
		{"Service Unavailable", "remote: Service Unavailable"},
		{"Failed to connect", "fatal: unable to connect to gitlab.com: Failed to connect to gitlab.com port 443"},
		{"Temporary failure in name resolution", "fatal: unable to look up gitlab.com (port 9418) (Temporary failure in name resolution)"},
		{"Remote disconnected", "fatal: remote error: The remote unexpectedly disconnected."},
		{"early EOF", "error: RPC failed; curl 18 transfer closed with outstanding read data remaining\nfatal: early EOF"},
		{"index-pack failed", "fatal: index-pack failed"},
		{"unexpected disconnect", "ssh: disconnect: Connection reset by peer"},
		{"error reading from remote", "fatal: error reading from remote repository"},
		{"connection closed", "fatal: The remote end hung up unexpectedly"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isTransientGitError(errors.New("exit status 128"), tc.stderr)
			assert.True(t, result, "stderr: %s", tc.stderr)
		})
	}
}

func TestIsTransientGitError_PermanentPatterns(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
	}{
		{"authentication failed", "fatal: Authentication failed for 'https://gitlab.com/repo.git'"},
		{"authentication required", "remote: HTTP Basic: Access denied\nfatal: Authentication failed"},
		{"permission denied publickey", "git@gitlab.com: Permission denied (publickey)."},
		{"repository not found", "remote: The project you were looking for could not be found.\nfatal: repository 'https://gitlab.com/org/repo.git/' not found"},
		{"not a git repo", "fatal: not a git repository (or any of the parent directories): .git"},
		{"does not appear to be a git repo", "fatal: repository 'https://gitlab.com/org/missing.git/' does not appear to be a git repository"},
		{"could not read username", "fatal: could not read Username for 'https://gitlab.com': terminal prompts disabled"},
		{"couldn't find remote ref", "fatal: couldn't find remote ref main"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isTransientGitError(errors.New("exit status 128"), tc.stderr)
			assert.False(t, result, "stderr: %s", tc.stderr)
		})
	}
}

func TestIsTransientGitError_UnknownErrorDefaultsTransient(t *testing.T) {
	result := isTransientGitError(errors.New("exit status 128"), "some unknown git error message")
	assert.True(t, result, "unknown errors should default to transient (retryable)")
}

func TestIsTransientGitError_NilErrIsNotTransient(t *testing.T) {
	result := isTransientGitError(nil, "")
	assert.False(t, result, "nil error should not be transient")
}

func TestIsTransientGitError_CaseInsensitive(t *testing.T) {
	result := isTransientGitError(errors.New("error"), "CONNECTION REFUSED")
	assert.True(t, result, "pattern matching should be case-insensitive")
}

func TestNewGitBackoff(t *testing.T) {
	b := newGitBackoff()
	assert.Equal(t, 2*time.Second, b.InitialInterval)
	assert.Equal(t, 2.0, b.Multiplier)
	assert.Equal(t, 30*time.Second, b.MaxInterval)
	assert.Equal(t, 2*time.Minute, b.MaxElapsedTime)
	assert.Equal(t, 0.5, b.RandomizationFactor)
}
