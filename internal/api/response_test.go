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

	var env envelope
	err := json.NewDecoder(w.Body).Decode(&env)
	require.NoError(t, err)
	assert.Nil(t, env.Error)
	assert.Equal(t, map[string]any{"key": "value"}, env.Data)
}

func TestRespondJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 204, nil)

	assert.Equal(t, 204, w.Code)

	var env envelope
	err := json.NewDecoder(w.Body).Decode(&env)
	require.NoError(t, err)
	assert.Nil(t, env.Error)
	assert.Nil(t, env.Data)
}

func TestRespondJSON_Array(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 200, []string{"a", "b"})

	var env envelope
	json.NewDecoder(w.Body).Decode(&env)
	arr, ok := env.Data.([]any)
	assert.True(t, ok)
	assert.Len(t, arr, 2)
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, 400, "INVALID_JSON", "bad request")

	assert.Equal(t, 400, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var env envelope
	err := json.NewDecoder(w.Body).Decode(&env)
	require.NoError(t, err)
	assert.Nil(t, env.Data)
	require.NotNil(t, env.Error)
	assert.Equal(t, "INVALID_JSON", env.Error.Code)
	assert.Equal(t, "bad request", env.Error.Message)
}

func TestRespondError_500(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, 500, "INTERNAL_ERROR", "server error")

	var env envelope
	json.NewDecoder(w.Body).Decode(&env)
	require.NotNil(t, env.Error)
	assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
	assert.Equal(t, "server error", env.Error.Message)
}

func TestRespondNoContent(t *testing.T) {
	w := httptest.NewRecorder()
	respondNoContent(w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}
