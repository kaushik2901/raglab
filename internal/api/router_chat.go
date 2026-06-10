package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openai/openai-go"
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

	messages := r.svc.buildMessages(body, results)

	var answer string
	completion, err := r.svc.generator.GenerateStream(req.Context(), openai.ChatCompletionNewParams{
		Messages:  messages,
		MaxTokens: openai.Int(int64(resolveMaxTokens(body))),
	}, func(token string) error {
		sendEvent("token", map[string]string{"token": token})
		answer += token
		return nil
	})
	if err != nil {
		sendEvent("error", map[string]string{"code": "GENERATION_FAILED", "message": err.Error()})
		return
	}

	if body.ConversationID != "" {
		r.svc.memory.Add(body.ConversationID, body.Query, answer)
	}

	sendEvent("done", map[string]any{
		"source_documents": sources,
		"tokens": TokenUsage{
			Prompt:     int(completion.Usage.PromptTokens),
			Completion: int(completion.Usage.CompletionTokens),
			Total:      int(completion.Usage.TotalTokens),
		},
		"latency_ms": time.Since(start).Milliseconds(),
	})
}
