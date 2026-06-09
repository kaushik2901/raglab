package memory

import (
	"sync"
	"time"
)

type Message struct {
	Role    string
	Content string
}

type Turn struct {
	User      Message
	Assistant Message
}

type Memory interface {
	Add(conversationID string, userMsg, assistantMsg string)
	Get(conversationID string) []Turn
	Clear(conversationID string)
}

type RingBuffer struct {
	mu           sync.Mutex
	maxTurns     int
	buffer       map[string][]Turn
	lastAccessed map[string]time.Time
}

func NewRingBuffer(maxTurns int) *RingBuffer {
	if maxTurns <= 0 {
		maxTurns = 10
	}
	return &RingBuffer{
		maxTurns:     maxTurns,
		buffer:       make(map[string][]Turn),
		lastAccessed: make(map[string]time.Time),
	}
}

func (r *RingBuffer) Add(conversationID string, userMsg, assistantMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	turns := r.buffer[conversationID]
	turn := Turn{
		User:      Message{Role: "user", Content: userMsg},
		Assistant: Message{Role: "assistant", Content: assistantMsg},
	}
	turns = append(turns, turn)

	if len(turns) > r.maxTurns {
		turns = turns[len(turns)-r.maxTurns:]
	}

	r.buffer[conversationID] = turns
	r.lastAccessed[conversationID] = time.Now()
}

func (r *RingBuffer) Get(conversationID string) []Turn {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastAccessed[conversationID] = time.Now()

	turns := r.buffer[conversationID]
	result := make([]Turn, len(turns))
	copy(result, turns)
	return result
}

func (r *RingBuffer) Clear(conversationID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.buffer, conversationID)
	delete(r.lastAccessed, conversationID)
}

func (r *RingBuffer) Purge(threshold time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-threshold)
	for id, last := range r.lastAccessed {
		if last.Before(cutoff) {
			delete(r.buffer, id)
			delete(r.lastAccessed, id)
		}
	}
}
