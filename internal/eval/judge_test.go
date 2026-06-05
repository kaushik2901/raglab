package eval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
)

func respondWithJSON(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":      "judge-123",
		"object":  "chat.completion",
		"created": 1700000000,
		"model":   "gpt-4o",
		"choices": []map[string]any{
			{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
			},
		},
	})
}

func TestJudgeAnswer_ValidScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, `{"score": 0.85, "reasoning": "correct"}`)
	}))
	defer srv.Close()

	gen := generator.NewOpenAI(srv.URL, "", "gpt-4o")

	score, err := JudgeAnswer(context.Background(), gen, "What is SSH?", "SSH is a network protocol", "SSH is a protocol", "SSH is a protocol")
	require.NoError(t, err)
	assert.InDelta(t, 0.85, score, 0.001)
}

func TestJudgeAnswer_PerfectScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, `{"score": 1.0, "reasoning": "exact match"}`)
	}))
	defer srv.Close()

	gen := generator.NewOpenAI(srv.URL, "", "gpt-4o")

	score, err := JudgeAnswer(context.Background(), gen, "q", "ctx", "a", "a")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, score, 0.001)
}

func TestJudgeAnswer_ZeroScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, `{"score": 0.0, "reasoning": "completely wrong"}`)
	}))
	defer srv.Close()

	gen := generator.NewOpenAI(srv.URL, "", "gpt-4o")

	score, err := JudgeAnswer(context.Background(), gen, "q", "ctx", "a", "wrong")
	require.NoError(t, err)
	assert.InDelta(t, 0.0, score, 0.001)
}

func TestJudgeAnswer_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	gen := generator.NewOpenAI(srv.URL, "", "gpt-4o")

	_, err := JudgeAnswer(context.Background(), gen, "q", "ctx", "a", "b")
	require.Error(t, err)
}

func TestJudgeAnswer_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "judge-123",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "gpt-4o",
			"choices": []map[string]any{},
		})
	}))
	defer srv.Close()

	gen := generator.NewOpenAI(srv.URL, "", "gpt-4o")

	_, err := JudgeAnswer(context.Background(), gen, "q", "ctx", "a", "b")
	require.Error(t, err)
}

func TestJudgeAnswer_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, `not json`)
	}))
	defer srv.Close()

	gen := generator.NewOpenAI(srv.URL, "", "gpt-4o")

	_, err := JudgeAnswer(context.Background(), gen, "q", "ctx", "a", "b")
	require.Error(t, err)
}

func TestJudgeAnswer_ScoreOutOfRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, `{"score": 1.5, "reasoning": "out of range"}`)
	}))
	defer srv.Close()

	gen := generator.NewOpenAI(srv.URL, "", "gpt-4o")

	_, err := JudgeAnswer(context.Background(), gen, "q", "ctx", "a", "b")
	require.Error(t, err)
}

func TestJudgeAnswer_MissingScoreField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(w, `{"reasoning": "no score"}`)
	}))
	defer srv.Close()

	gen := generator.NewOpenAI(srv.URL, "", "gpt-4o")

	score, err := JudgeAnswer(context.Background(), gen, "q", "ctx", "a", "b")
	require.NoError(t, err)
	assert.Equal(t, 0.0, score)
}
