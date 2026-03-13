package dean

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConversationNotFound = errors.New("conversation not found")
	ErrConversationArchived = errors.New("conversation is archived")
	ErrMessageNotFound      = errors.New("message not found")
)

type Repository struct {
	db *sql.DB
}

type Conversation struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Title         string     `json:"title"`
	Summary       string     `json:"summary"`
	Status        string     `json:"status"`
	LastMessageAt *time.Time `json:"last_message_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Message struct {
	ID                string    `json:"id"`
	ConversationID    string    `json:"conversation_id"`
	UserID            string    `json:"user_id,omitempty"`
	Sequence          int64     `json:"sequence"`
	Role              string    `json:"role"`
	Status            string    `json:"status"`
	Provider          string    `json:"provider"`
	Model             string    `json:"model"`
	Content           string    `json:"content"`
	FinishReason      string    `json:"finish_reason"`
	ProviderRequestID string    `json:"provider_request_id"`
	ErrorText         string    `json:"error_text"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CreateConversationInput struct {
	UserID    string
	Title     string
	Summary   string
	CreatedBy string
}

type CreateUserMessageInput struct {
	UserID         string
	ConversationID string
	Content        string
	Provider       string
	Model          string
	CreatedBy      string
}

type UpdateAssistantMessageInput struct {
	ConversationID string
	MessageID      string
	Status         string
	Content        string
	FinishReason   string
	ErrorText      string
	UpdatedBy      string
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListConversationsForUser(ctx context.Context, userID string, limit, offset int) ([]Conversation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, summary, status, last_message_at, created_at, updated_at
		FROM dean_conversations
		WHERE user_id = ?
		ORDER BY COALESCE(last_message_at, created_at) DESC, created_at DESC
		LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Conversation, 0)
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, conversation)
	}
	return out, rows.Err()
}

func (r *Repository) CreateConversation(ctx context.Context, input CreateConversationInput) (Conversation, error) {
	now := time.Now().UTC()
	id := uuid.New().String()
	createdBy := strings.TrimSpace(input.CreatedBy)
	if createdBy == "" {
		createdBy = strings.TrimSpace(input.UserID)
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO dean_conversations
			(id, user_id, title, summary, status, created_by, created_at, updated_by, updated_at)
		 VALUES (?, ?, ?, ?, 'active', ?, ?, ?, ?)`,
		id,
		input.UserID,
		strings.TrimSpace(input.Title),
		strings.TrimSpace(input.Summary),
		createdBy,
		now,
		createdBy,
		now,
	)
	if err != nil {
		return Conversation{}, err
	}
	return r.GetConversationForUser(ctx, id, input.UserID)
}

func (r *Repository) GetConversationForUser(ctx context.Context, conversationID, userID string) (Conversation, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, user_id, title, summary, status, last_message_at, created_at, updated_at
		 FROM dean_conversations
		 WHERE id = ? AND user_id = ?`,
		conversationID, userID,
	)
	conversation, err := scanConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, ErrConversationNotFound
	}
	return conversation, err
}

func (r *Repository) ListMessagesForConversation(ctx context.Context, conversationID string, limit, offset int) ([]Message, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, conversation_id, user_id, sequence, role, status, provider, model, content, finish_reason, provider_request_id, error_text, created_at, updated_at
		FROM dean_messages
		WHERE conversation_id = ?
		ORDER BY sequence ASC, created_at ASC
		LIMIT ? OFFSET ?`,
		conversationID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Message, 0)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	return out, rows.Err()
}

func (r *Repository) ListRecentMessagesForConversation(ctx context.Context, conversationID string, limit int) ([]Message, error) {
	if limit < 1 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, conversation_id, user_id, sequence, role, status, provider, model, content, finish_reason, provider_request_id, error_text, created_at, updated_at
		FROM dean_messages
		WHERE conversation_id = ?
		ORDER BY sequence DESC
		LIMIT ?`,
		conversationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reversed := make([]Message, 0)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		reversed = append(reversed, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	return reversed, nil
}

func (r *Repository) CreateUserMessageAndAssistantPlaceholder(ctx context.Context, input CreateUserMessageInput) (Message, Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, Message{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var status string
	err = tx.QueryRowContext(
		ctx,
		"SELECT status FROM dean_conversations WHERE id = ? AND user_id = ?",
		input.ConversationID, input.UserID,
	).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, Message{}, ErrConversationNotFound
	}
	if err != nil {
		return Message{}, Message{}, err
	}
	if status != "active" {
		return Message{}, Message{}, ErrConversationArchived
	}

	var nextSequence int64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COALESCE(MAX(sequence), -1) + 1 FROM dean_messages WHERE conversation_id = ?",
		input.ConversationID,
	).Scan(&nextSequence); err != nil {
		return Message{}, Message{}, err
	}

	now := time.Now().UTC()
	createdBy := strings.TrimSpace(input.CreatedBy)
	if createdBy == "" {
		createdBy = strings.TrimSpace(input.UserID)
	}

	userMessage := Message{
		ID:             uuid.New().String(),
		ConversationID: input.ConversationID,
		UserID:         input.UserID,
		Sequence:       nextSequence,
		Role:           "user",
		Status:         "completed",
		Content:        strings.TrimSpace(input.Content),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO dean_messages
			(id, conversation_id, user_id, sequence, role, status, provider, model, content, finish_reason, provider_request_id, error_text, created_by, created_at, updated_by, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, '', '', ?, '', '', '', ?, ?, ?, ?)`,
		userMessage.ID,
		userMessage.ConversationID,
		userMessage.UserID,
		userMessage.Sequence,
		userMessage.Role,
		userMessage.Status,
		userMessage.Content,
		createdBy,
		now,
		createdBy,
		now,
	); err != nil {
		return Message{}, Message{}, err
	}

	assistantMessage := Message{
		ID:             uuid.New().String(),
		ConversationID: input.ConversationID,
		Sequence:       nextSequence + 1,
		Role:           "assistant",
		Status:         "pending",
		Provider:       strings.TrimSpace(input.Provider),
		Model:          strings.TrimSpace(input.Model),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO dean_messages
			(id, conversation_id, user_id, sequence, role, status, provider, model, content, finish_reason, provider_request_id, error_text, created_by, created_at, updated_by, updated_at)
		 VALUES (?, ?, NULL, ?, ?, ?, ?, ?, '', '', '', '', ?, ?, ?, ?)`,
		assistantMessage.ID,
		assistantMessage.ConversationID,
		assistantMessage.Sequence,
		assistantMessage.Role,
		assistantMessage.Status,
		assistantMessage.Provider,
		assistantMessage.Model,
		createdBy,
		now,
		createdBy,
		now,
	); err != nil {
		return Message{}, Message{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		"UPDATE dean_conversations SET last_message_at = ?, updated_by = ?, updated_at = ? WHERE id = ?",
		now,
		createdBy,
		now,
		input.ConversationID,
	); err != nil {
		return Message{}, Message{}, err
	}

	if err := tx.Commit(); err != nil {
		return Message{}, Message{}, err
	}

	return userMessage, assistantMessage, nil
}

func (r *Repository) GetAssistantMessageForUser(ctx context.Context, conversationID, messageID, userID string) (Message, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT m.id, m.conversation_id, m.user_id, m.sequence, m.role, m.status, m.provider, m.model, m.content, m.finish_reason, m.provider_request_id, m.error_text, m.created_at, m.updated_at
		 FROM dean_messages m
		 JOIN dean_conversations c ON c.id = m.conversation_id
		 WHERE c.user_id = ? AND c.id = ? AND m.id = ? AND m.role = 'assistant'`,
		userID, conversationID, messageID,
	)
	message, err := scanMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrMessageNotFound
	}
	return message, err
}

func (r *Repository) UpdateAssistantMessage(ctx context.Context, input UpdateAssistantMessageInput) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE dean_messages
		 SET status = ?, content = ?, finish_reason = ?, error_text = ?, updated_by = ?, updated_at = ?
		 WHERE id = ? AND conversation_id = ? AND role = 'assistant'`,
		input.Status,
		input.Content,
		input.FinishReason,
		input.ErrorText,
		input.UpdatedBy,
		now,
		input.MessageID,
		input.ConversationID,
	)
	return err
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanConversation(s scanner) (Conversation, error) {
	var conversation Conversation
	var lastMessageAt sql.NullTime
	if err := s.Scan(
		&conversation.ID,
		&conversation.UserID,
		&conversation.Title,
		&conversation.Summary,
		&conversation.Status,
		&lastMessageAt,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	); err != nil {
		return Conversation{}, err
	}
	if lastMessageAt.Valid {
		last := lastMessageAt.Time
		conversation.LastMessageAt = &last
	}
	return conversation, nil
}

func scanMessage(s scanner) (Message, error) {
	var message Message
	var nullableUserID sql.NullString
	if err := s.Scan(
		&message.ID,
		&message.ConversationID,
		&nullableUserID,
		&message.Sequence,
		&message.Role,
		&message.Status,
		&message.Provider,
		&message.Model,
		&message.Content,
		&message.FinishReason,
		&message.ProviderRequestID,
		&message.ErrorText,
		&message.CreatedAt,
		&message.UpdatedAt,
	); err != nil {
		return Message{}, err
	}
	if nullableUserID.Valid {
		message.UserID = nullableUserID.String
	}
	return message, nil
}
