package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ChatRouter struct {
	svc *ChatService
}

func NewChatRouter(svc *ChatService) *ChatRouter {
	return &ChatRouter{svc: svc}
}

func (r *ChatRouter) Register(mux chi.Router) {
	mux.Post("/", r.chatHandler)
	mux.Post("/stream", r.chatStreamHandler)
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

	results, sources, err := r.svc.retrieveSources(req.Context(), body)
	if err != nil {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)
		flusher.Flush()
		r.writeStreamPart(w, flusher, map[string]any{
			"type": "error", "errorText": fmt.Sprintf("RETRIEVAL_FAILED: %s", err.Error()),
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher.Flush()

	for _, src := range sources {
		r.writeStreamPart(w, flusher, map[string]any{
			"type":      "source-document",
			"sourceId":  src.DocumentPath,
			"mediaType": "text/markdown",
			"title":     src.DocumentPath,
			"filename":  src.DocumentPath,
		})
	}

	msgID := "text-" + uuid.NewString()
	r.writeStreamPart(w, flusher, map[string]any{
		"type": "text-start",
		"id":   msgID,
	})

	_, err = r.svc.ChatStream(req.Context(), body, results, sources, func(token string) error {
		r.writeStreamPart(w, flusher, map[string]any{
			"type":  "text-delta",
			"id":    msgID,
			"delta": token,
		})
		return nil
	})
	if err != nil {
		r.writeStreamPart(w, flusher, map[string]any{
			"type": "error", "errorText": fmt.Sprintf("GENERATION_FAILED: %s", err.Error()),
		})
		return
	}

	r.writeStreamPart(w, flusher, map[string]any{
		"type": "text-end",
		"id":   msgID,
	})

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (r *ChatRouter) writeStreamPart(w http.ResponseWriter, flusher http.Flusher, part any) {
	jsonData, _ := json.Marshal(part)
	fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
	flusher.Flush()
}
