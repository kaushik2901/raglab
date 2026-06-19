package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ChatRouter struct {
	svc  *ChatService
	repo *ChatRepository
}

func NewChatRouter(svc *ChatService, repo *ChatRepository) *ChatRouter {
	return &ChatRouter{svc: svc, repo: repo}
}

func (r *ChatRouter) Register(mux chi.Router) {
	mux.Post("/", r.chatHandler)
	mux.Post("/stream", r.chatStreamHandler)
	mux.Get("/conversations/{id}", r.getConversationHandler)
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

	w.Header().Set("X-Conversation-ID", resp.ConversationID)
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

	msgID := uuid.NewString()
	r.writeStreamPart(w, flusher, map[string]any{
		"type": "text-start",
		"id":   msgID,
	})

	start := time.Now()

	chatResp, err := r.svc.ChatStream(req.Context(), body, results, sources, func(token string) error {
		r.writeStreamPart(w, flusher, map[string]any{
			"type":  "text-delta",
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

	finishPayload := map[string]any{
		"type":         "text-end",
		"finishReason": "stop",
	}
	if chatResp.TokenUsage.Total > 0 {
		finishPayload["usage"] = map[string]int{
			"promptTokens":     chatResp.TokenUsage.Prompt,
			"completionTokens": chatResp.TokenUsage.Completion,
		}
	}
	r.writeStreamPart(w, flusher, finishPayload)

	w.Header().Set("X-Conversation-ID", chatResp.ConversationID)
	w.Header().Set("X-Latency-Ms", fmt.Sprintf("%d", time.Since(start).Milliseconds()))
}

func (r *ChatRouter) writeStreamPart(w http.ResponseWriter, flusher http.Flusher, part any) {
	jsonData, _ := json.Marshal(part)
	fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
	flusher.Flush()
}

func (r *ChatRouter) getConversationHandler(w http.ResponseWriter, req *http.Request) {
	idStr := chi.URLParam(req, "id")
	convID, err := uuid.Parse(idStr)
	if err != nil {
		respondProblem(w, 400, "Invalid Parameter", "invalid conversation id")
		return
	}

	conv, err := r.repo.GetConversation(req.Context(), convID)
	if err != nil {
		respondProblem(w, 404, "Not Found", "conversation not found")
		return
	}

	respondJSON(w, 200, conv)
}
