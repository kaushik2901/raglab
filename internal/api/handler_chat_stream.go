package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go"
)

func (s *Server) chatStreamHandler(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, 400, "INVALID_JSON", "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		respondError(w, 400, "INVALID_PARAMETER", err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, 500, "INTERNAL_ERROR", "streaming not supported")
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

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	results, err := s.chat.retriever.Retrieve(r.Context(), req.Tag, req.Query, topK)
	if err != nil {
		sendEvent("error", map[string]string{"code": "RETRIEVAL_FAILED", "message": err.Error()})
		return
	}

	sources := make([]SourceDoc, len(results))
	for i, r := range results {
		sources[i] = SourceDoc{DocumentPath: r.DocumentPath, Score: r.Score}
	}
	sendEvent("retrieval", map[string]any{"results": sources})

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a helpful assistant that answers questions based on the provided context. If the context does not contain enough information to answer, say so."),
	}

	if req.ConversationID != "" {
		for _, turn := range s.chat.memory.Get(req.ConversationID) {
			messages = append(messages, openai.UserMessage(turn.User.Content))
			messages = append(messages, openai.AssistantMessage(turn.Assistant.Content))
		}
	}

	var contextParts []string
	for _, r := range results {
		contextParts = append(contextParts, fmt.Sprintf("Document: %s\n%s", r.DocumentPath, r.Content))
	}
	userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", strings.Join(contextParts, "\n\n"), req.Query)
	messages = append(messages, openai.UserMessage(userPrompt))

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	var answer string
	completion, err := s.chat.generator.GenerateStream(r.Context(), openai.ChatCompletionNewParams{
		Messages:  messages,
		MaxTokens: openai.Int(int64(maxTokens)),
	}, func(token string) error {
		sendEvent("token", map[string]string{"token": token})
		answer += token
		return nil
	})
	if err != nil {
		sendEvent("error", map[string]string{"code": "GENERATION_FAILED", "message": err.Error()})
		return
	}

	if req.ConversationID != "" {
		s.chat.memory.Add(req.ConversationID, req.Query, answer)
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
