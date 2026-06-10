package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArtifactListHandler_Empty(t *testing.T) {
	origDir := "artifacts"
	backup := "_artifacts_backup"
	os.Rename(origDir, backup)
	defer os.Rename(backup, origDir)

	os.MkdirAll(origDir, 0755)
	defer os.RemoveAll(origDir)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/artifacts", nil)

	ar := &ArtifactRouter{artifactsDir: origDir}
	ar.listHandler(w, r)

	assert.Equal(t, 200, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	artifacts := data["artifacts"].([]any)
	assert.Empty(t, artifacts)
}

func TestArtifactListHandler_WithFiles(t *testing.T) {
	dir := t.TempDir()

	ar := &ArtifactRouter{artifactsDir: dir}

	os.MkdirAll(filepath.Join(dir, "preprocessing", "tag1", "output"), 0755)
	os.WriteFile(filepath.Join(dir, "preprocessing", "tag1", "output", "test.md"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "preprocessing", "tag1", "output", "other.txt"), []byte("world"), 0644)

	os.MkdirAll(filepath.Join(dir, "indexing", "tag2"), 0755)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/artifacts", nil)

	ar.listHandler(w, r)

	assert.Equal(t, 200, w.Code)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	artifacts := data["artifacts"].([]any)
	assert.Len(t, artifacts, 2)

	foundPre := false
	foundIdx := false
	for _, a := range artifacts {
		entry := a.(map[string]any)
		switch entry["type"] {
		case "preprocessing":
			foundPre = true
			assert.Equal(t, "tag1", entry["tag"])
			assert.Equal(t, float64(1), entry["file_count"])
		case "indexing":
			foundIdx = true
			assert.Equal(t, "tag2", entry["tag"])
			assert.Nil(t, entry["file_count"])
		}
	}
	assert.True(t, foundPre)
	assert.True(t, foundIdx)
}

func TestArtifactListHandler_FilterByType(t *testing.T) {
	dir := t.TempDir()

	ar := &ArtifactRouter{artifactsDir: dir}

	os.MkdirAll(filepath.Join(dir, "preprocessing", "tag1", "output"), 0755)
	os.MkdirAll(filepath.Join(dir, "indexing", "tag2"), 0755)
	os.WriteFile(filepath.Join(dir, "preprocessing", "tag1", "output", "a.md"), []byte("x"), 0644)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/artifacts?type=preprocessing", nil)

	ar.listHandler(w, r)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	artifacts := data["artifacts"].([]any)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "preprocessing", artifacts[0].(map[string]any)["type"])
}

func TestCountFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "d.md"), []byte("d"), 0644)

	assert.Equal(t, 3, countFiles(dir, ".md"))
	assert.Equal(t, 1, countFiles(dir, ".txt"))
}
