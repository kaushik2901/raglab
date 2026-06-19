package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ChatRepository struct {
	pool *pgxpool.Pool
}

func NewChatRepository(pool *pgxpool.Pool) *ChatRepository {
	return &ChatRepository{pool: pool}
}

type DBChatMessage struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Role           string
	Content        string
	Sources        json.RawMessage
	TokenUsage     json.RawMessage
	CreatedAt      time.Time
}

type ConversationWithMessages struct {
	ID        uuid.UUID       `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Messages  []DBChatMessage `json:"messages"`
}

func (r *ChatRepository) CreateConversation(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx,
		`INSERT INTO chat_conversations DEFAULT VALUES RETURNING id`,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create conversation: %w", err)
	}
	return id, nil
}

func (r *ChatRepository) GetOrCreateConversation(ctx context.Context, conversationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO chat_conversations (id) VALUES ($1) ON CONFLICT DO NOTHING`,
		conversationID,
	)
	if err != nil {
		return fmt.Errorf("get or create conversation: %w", err)
	}
	return nil
}

func (r *ChatRepository) AddMessage(ctx context.Context, conversationID uuid.UUID, role, content string, sources, tokenUsage json.RawMessage) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO chat_messages (conversation_id, role, content, sources, token_usage)
		 VALUES ($1, $2, $3, $4, $5)`,
		conversationID, role, content, sources, tokenUsage,
	)
	if err != nil {
		return fmt.Errorf("add message: %w", err)
	}

	_, err = r.pool.Exec(ctx,
		`UPDATE chat_conversations SET updated_at = NOW() WHERE id = $1`,
		conversationID,
	)
	if err != nil {
		return fmt.Errorf("update conversation timestamp: %w", err)
	}
	return nil
}

func (r *ChatRepository) GetMessages(ctx context.Context, conversationID uuid.UUID) ([]DBChatMessage, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, conversation_id, role, content, sources, token_usage, created_at
		 FROM chat_messages
		 WHERE conversation_id = $1
		 ORDER BY created_at ASC`,
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []DBChatMessage
	for rows.Next() {
		var m DBChatMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Sources, &m.TokenUsage, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return messages, nil
}

func (r *ChatRepository) GetConversation(ctx context.Context, conversationID uuid.UUID) (*ConversationWithMessages, error) {
	var conv ConversationWithMessages
	err := r.pool.QueryRow(ctx,
		`SELECT id, created_at FROM chat_conversations WHERE id = $1`,
		conversationID,
	).Scan(&conv.ID, &conv.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	messages, err := r.GetMessages(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	conv.Messages = messages
	if conv.Messages == nil {
		conv.Messages = []DBChatMessage{}
	}
	return &conv, nil
}
