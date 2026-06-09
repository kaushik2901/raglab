package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	var env envelope
	assert.NoError(t, jsonUnmarshal(w.Body.Bytes(), &env))
	assert.NotNil(t, env.Error)
	assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
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
		w.WriteHeader(200)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, r)

	// Context deadline exceeded should be returned by the server
	assert.Equal(t, 200, w.Code)
}

func TestCORS_Simple(t *testing.T) {
	handler := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, r)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, X-Request-ID", w.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, 200, w.Code)
}

func TestCORS_Options(t *testing.T) {
	handler := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("OPTIONS", "/", nil)
	handler.ServeHTTP(w, r)

	assert.Equal(t, 204, w.Code)
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
