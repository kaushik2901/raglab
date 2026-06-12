package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDatasetDir(t *testing.T) (string, func()) {
	dir := t.TempDir()
	return dir, func() {}
}

func TestDatasetRouter_Upload_Success(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	body := "--boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"test.jsonl\"\r\nContent-Type: application/octet-stream\r\n\r\n{\"question\": \"q1\", \"expected\": \"a1\"}\n{\"question\": \"q2\", \"expected\": \"a2\"}\n\r\n--boundary--\r\n"
	req := httptest.NewRequest("POST", "/datasets", strings.NewReader(body))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 201, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "test.jsonl", data["name"])
	assert.Equal(t, float64(2), data["question_count"])
}

func TestDatasetRouter_Upload_InvalidExtension(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	body := "--boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"test.txt\"\r\nContent-Type: application/octet-stream\r\n\r\nsome content\r\n--boundary--\r\n"
	req := httptest.NewRequest("POST", "/datasets", strings.NewReader(body))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 400, rec.Code)
}

func TestDatasetRouter_List_Empty(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/datasets", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	datasets := data["datasets"].([]any)
	assert.Len(t, datasets, 0)
}

func TestDatasetRouter_List_WithFiles(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	err := os.WriteFile(filepath.Join(datasetsDir, "test.jsonl"), []byte("{\"q\":\"a\"}\n"), 0o644)
	require.NoError(t, err)

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/datasets", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	datasets := data["datasets"].([]any)
	assert.Len(t, datasets, 1)
}

func TestDatasetRouter_Download_Success(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	content := "{\"question\": \"q1\"}\n"
	err := os.WriteFile(filepath.Join(datasetsDir, "test.jsonl"), []byte(content), 0o644)
	require.NoError(t, err)

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/datasets/test.jsonl", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "application/x-ndjson", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "test.jsonl")
}

func TestDatasetRouter_Download_NotFound(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/datasets/non-existent.jsonl", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 404, rec.Code)
}

func TestDatasetRouter_Delete_Success(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	err := os.WriteFile(filepath.Join(datasetsDir, "test.jsonl"), []byte("{}"), 0o644)
	require.NoError(t, err)

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("DELETE", "/datasets/test.jsonl", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "test.jsonl", data["deleted"])
}

func TestDatasetRouter_Delete_NotFound(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("DELETE", "/datasets/non-existent.jsonl", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 404, rec.Code)
}

func TestCountJSONLLines_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	err := os.WriteFile(path, []byte("{\"a\":1}\n{\"a\":2}\n"), 0o644)
	require.NoError(t, err)

	count, err := countJSONLLines(path)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestCountJSONLLines_EmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	err := os.WriteFile(path, []byte("{\"a\":1}\n\n\n{\"a\":2}\n"), 0o644)
	require.NoError(t, err)

	count, err := countJSONLLines(path)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestCountJSONLLines_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	err := os.WriteFile(path, []byte("not valid json\n"), 0o644)
	require.NoError(t, err)

	_, err = countJSONLLines(path)
	assert.ErrorContains(t, err, "invalid JSON")
}

func TestDatasetRouter_List_IgnoresNonJSONL(t *testing.T) {
	datasetsDir, cleanup := setupDatasetDir(t)
	defer cleanup()

	os.WriteFile(filepath.Join(datasetsDir, "test.txt"), []byte("content"), 0o644)
	os.Mkdir(filepath.Join(datasetsDir, "subdir"), 0o755)

	r := NewDatasetRouter(datasetsDir)
	mux := chi.NewRouter()
	r.Register(mux)

	req := httptest.NewRequest("GET", "/datasets", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]any)
	datasets := data["datasets"].([]any)
	assert.Len(t, datasets, 0)
}
