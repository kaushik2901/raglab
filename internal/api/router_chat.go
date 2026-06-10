package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type ChatRouter struct {
	svc *ChatService
}

func NewChatRouter(svc *ChatService) *ChatRouter {
	return &ChatRouter{svc: svc}
}

func (r *ChatRouter) Register(mux chi.Router) {
	mux.Post("/chat", r.chatHandler)
	mux.Post("/chat/stream", r.chatStreamHandler)
}

func (r *ChatRouter) chatHandler(w http.ResponseWriter, req *http.Request) {
	var body ChatRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		respondProblem(w, 400, "Invalid Request Body", "invalid request body")
		return
	}
	if err := body.Validate(); err != nil {
		respondProblem(w, 400, "Invalid Parameter", err.Error())
		return
	}

	resp, err := r.svc.Chat(req.Context(), body)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	respondJSON(w, 200, resp)
}

func (r *ChatRouter) chatStreamHandler(w http.ResponseWriter, req *http.Request) {
	var body ChatRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		respondProblem(w, 400, "Invalid Request Body", "invalid request body")
		return
	}
	if err := body.Validate(); err != nil {
		respondProblem(w, 400, "Invalid Parameter", err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondProblem(w, 500, "Internal Server Error", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sendEvent := func(event string, data any) {
		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
		flusher.Flush()
	}

	start := time.Now()

	results, sources, err := r.svc.retrieveSources(req.Context(), body)
	if err != nil {
		sendEvent("error", map[string]string{"code": "RETRIEVAL_FAILED", "message": err.Error()})
		return
	}
	sendEvent("retrieval", map[string]any{"results": sources})

	resp, err := r.svc.ChatStream(req.Context(), body, results, sources, func(token string) error {
		sendEvent("token", map[string]string{"token": token})
		return nil
	})
	if err != nil {
		sendEvent("error", map[string]string{"code": "GENERATION_FAILED", "message": err.Error()})
		return
	}

	sendEvent("done", map[string]any{
		"source_documents": resp.SourceDocuments,
		"tokens":           resp.TokenUsage,
		"latency_ms":       time.Since(start).Milliseconds(),
	})
}
