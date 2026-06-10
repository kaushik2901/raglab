package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 200, map[string]string{"key": "value"})

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"key": "value"}, body["data"])
}

func TestRespondJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 204, nil)

	assert.Equal(t, 204, w.Code)

	var body map[string]any
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Nil(t, body["data"])
}

func TestRespondJSON_Array(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 200, []string{"a", "b"})

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	arr, ok := body["data"].([]any)
	assert.True(t, ok)
	assert.Len(t, arr, 2)
}

func TestRespondProblem(t *testing.T) {
	w := httptest.NewRecorder()
	respondProblem(w, 400, "Bad Request", "invalid request body")

	assert.Equal(t, 400, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var p ProblemDetail
	err := json.NewDecoder(w.Body).Decode(&p)
	require.NoError(t, err)
	assert.Equal(t, "/errors/bad-request", p.Type)
	assert.Equal(t, "Bad Request", p.Title)
	assert.Equal(t, 400, p.Status)
	assert.Equal(t, "invalid request body", p.Detail)
}

func TestRespondProblem_500(t *testing.T) {
	w := httptest.NewRecorder()
	respondProblem(w, 500, "Internal Server Error", "server error")

	var p ProblemDetail
	json.NewDecoder(w.Body).Decode(&p)
	assert.Equal(t, 500, p.Status)
	assert.Equal(t, "/errors/internal-server-error", p.Type)
}

func TestRespondNoContent(t *testing.T) {
	w := httptest.NewRecorder()
	respondNoContent(w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}
