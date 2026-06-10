package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRequestID_Generated(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Context().Value(ctxKeyRequestID)
		assert.NotEmpty(t, id)
		w.Header().Set("X-Request-ID", id.(string))
		w.WriteHeader(200)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, r)

	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

func TestRequestID_Propagated(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Context().Value(ctxKeyRequestID)
		assert.Equal(t, "client-id", id)
		w.WriteHeader(200)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Request-ID", "client-id")
	handler.ServeHTTP(w, r)

	assert.Equal(t, "client-id", w.Header().Get("X-Request-ID"))
}

func TestRecovery_Panic(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, r)

	assert.Equal(t, 500, w.Code)

	var p ProblemDetail
	assert.NoError(t, jsonUnmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, 500, p.Status)
	assert.Equal(t, "Internal Server Error", p.Title)
}

func TestRecovery_NoPanic(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, r)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

func TestTimeout_Exceeded(t *testing.T) {
	handler := Timeout(10 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		// Do NOT write header — the timeout middleware will write 503
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, r)

	assert.Equal(t, 503, w.Code)

	var p ProblemDetail
	assert.NoError(t, jsonUnmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, "/errors/request-timeout", p.Type)
}

func TestMaxBodySize_Exceeded(t *testing.T) {
	handler := MaxBodySize(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 100)
		_, err := r.Body.Read(buf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(200)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader("this body is too long"))
	r.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
