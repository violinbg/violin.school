package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/violinbg/violin.school/internal/ai"
	"github.com/violinbg/violin.school/internal/dean"
)

func registerCommunicationRoutes(protected *gin.RouterGroup, db *sql.DB) {
	repository := dean.NewRepository(db)
	streamProvider := dean.NewXAIStreamProvider(ai.NewXAIClient(), func(ctx context.Context) (string, string, error) {
		service, err := aiService(db)
		if err != nil {
			return "", "", err
		}
		return service.ResolveStreamingConfig(ctx)
	})

	protected.GET("/communications/conversations", handleListConversations(repository))
	protected.POST("/communications/conversations", handleCreateConversation(repository))
	protected.GET("/communications/conversations/:conversationID/messages", handleListMessages(repository))
	protected.POST("/communications/conversations/:conversationID/messages", handlePostMessage(repository))
	protected.POST("/communications/conversations/:conversationID/archive", handleArchiveConversation(repository))
	protected.POST("/communications/conversations/:conversationID/restore", handleRestoreConversation(repository))
	protected.DELETE("/communications/conversations/:conversationID", handleDeleteConversation(repository))
	protected.POST("/communications/conversations/:conversationID/convert-to-mail", handleConvertConversationToMail(repository, db))
	protected.GET("/communications/conversations/:conversationID/messages/:messageID/stream", handleStreamMessage(repository, streamProvider, db))
}

func handleListConversations(repository *dean.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		limit, ok := parseBoundedIntQuery(c, "limit", 20, 1, 100)
		if !ok {
			return
		}
		offset, ok := parseBoundedIntQuery(c, "offset", 0, 0, 10000)
		if !ok {
			return
		}

		channelType := firstNonEmpty(strings.TrimSpace(c.Query("channel_type")), "mail_thread")
		contextKey := strings.TrimSpace(c.Query("context_key"))
		recipientKind := strings.TrimSpace(c.Query("recipient_kind"))
		status := strings.TrimSpace(c.Query("status"))

		conversations, err := repository.ListConversationsForUser(c.Request.Context(), dean.ListConversationsInput{
			UserID:        userID,
			ChannelType:   channelType,
			ContextKey:    contextKey,
			RecipientKind: recipientKind,
			Status:        status,
			Limit:         limit,
			Offset:        offset,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list conversations"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"conversations": conversations})
	}
}

func handleCreateConversation(repository *dean.Repository) gin.HandlerFunc {
	type request struct {
		ChannelType   string `json:"channel_type"`
		ContextKey    string `json:"context_key" binding:"max=200"`
		RecipientKind string `json:"recipient_kind" binding:"max=100"`
		RecipientID   string `json:"recipient_id" binding:"max=200"`
		Title         string `json:"title" binding:"max=200"`
		Summary       string `json:"summary" binding:"max=2000"`
	}

	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			if errors.Is(err, io.EOF) {
				req = request{}
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}

		channelType := firstNonEmpty(strings.TrimSpace(req.ChannelType), "mail_thread")
		title := strings.TrimSpace(req.Title)
		if channelType == "mail_thread" && title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title is required for mail conversations"})
			return
		}

		conversation, err := repository.CreateConversation(c.Request.Context(), dean.CreateConversationInput{
			UserID:        userID,
			ChannelType:   channelType,
			ContextKey:    strings.TrimSpace(req.ContextKey),
			RecipientKind: firstNonEmpty(strings.TrimSpace(req.RecipientKind), "dean_office"),
			RecipientID:   firstNonEmpty(strings.TrimSpace(req.RecipientID), "dean.taskford"),
			Title:         title,
			Summary:       strings.TrimSpace(req.Summary),
			CreatedBy:     userID,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create conversation"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"conversation": conversation})
	}
}

func handleListMessages(repository *dean.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		conversationID := strings.TrimSpace(c.Param("conversationID"))
		if conversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id is required"})
			return
		}

		conversation, err := repository.GetConversationForUser(c.Request.Context(), conversationID, userID)
		if errors.Is(err, dean.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load conversation"})
			return
		}

		limit, ok := parseBoundedIntQuery(c, "limit", 100, 1, 200)
		if !ok {
			return
		}
		offset, ok := parseBoundedIntQuery(c, "offset", 0, 0, 20000)
		if !ok {
			return
		}

		messages, err := repository.ListMessagesForConversation(c.Request.Context(), conversation.ID, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list messages"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"conversation": conversation,
			"messages":     messages,
		})
	}
}

func handlePostMessage(repository *dean.Repository) gin.HandlerFunc {
	type request struct {
		Content  string `json:"content" binding:"required"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}

	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		conversationID := strings.TrimSpace(c.Param("conversationID"))
		if conversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id is required"})
			return
		}

		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.Content = strings.TrimSpace(req.Content)
		if req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
			return
		}
		if len(req.Content) > 20000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "content must be 20000 characters or less"})
			return
		}

		userMessage, assistantMessage, err := repository.CreateUserMessageAndAssistantPlaceholder(c.Request.Context(), dean.CreateUserMessageInput{
			UserID:         userID,
			ConversationID: conversationID,
			Content:        req.Content,
			Provider:       strings.TrimSpace(req.Provider),
			Model:          strings.TrimSpace(req.Model),
			CreatedBy:      userID,
		})
		if errors.Is(err, dean.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
		if errors.Is(err, dean.ErrConversationArchived) {
			c.JSON(http.StatusConflict, gin.H{"error": "conversation is archived"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create conversation messages"})
			return
		}

		streamURL := fmt.Sprintf("/api/v1/communications/conversations/%s/messages/%s/stream", conversationID, assistantMessage.ID)
		c.JSON(http.StatusAccepted, gin.H{
			"user_message":      userMessage,
			"assistant_message": assistantMessage,
			"stream": gin.H{
				"url":          streamURL,
				"method":       http.MethodGet,
				"content_type": "text/event-stream",
			},
		})
	}
}

func handleConvertConversationToMail(repository *dean.Repository, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		conversationID := strings.TrimSpace(c.Param("conversationID"))
		if conversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id is required"})
			return
		}

		conversation, err := repository.ConvertConversationToMailForUser(c.Request.Context(), conversationID, userID, userID)
		if errors.Is(err, dean.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
		if errors.Is(err, dean.ErrConversationNotContextChat) {
			c.JSON(http.StatusConflict, gin.H{"error": "conversation is not a context chat"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not convert conversation"})
			return
		}

		if shouldGenerateConversationSubject(conversation.Title) {
			if usageService, err := aiService(db); err == nil {
				contextMessages, err := repository.ListRecentMessagesForConversation(c.Request.Context(), conversationID, 20)
				if err == nil {
					userLanguage := loadUserLanguage(c.Request.Context(), db, userID)
					subject, err := usageService.GenerateConversationSubject(c.Request.Context(), ai.GenerateConversationSubjectInput{
						UserID:   userID,
						Locale:   userLanguage,
						Messages: buildSubjectGenerationPromptMessages(conversation, contextMessages),
						MaxChars: 40,
					})
					if err == nil {
						if updatedConversation, err := repository.UpdateConversationTitleForUser(c.Request.Context(), conversationID, userID, subject, userID); err == nil {
							conversation = updatedConversation
						}
					}
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{"conversation": conversation})
	}
}

func handleArchiveConversation(repository *dean.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		conversationID := strings.TrimSpace(c.Param("conversationID"))
		if conversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id is required"})
			return
		}

		conversation, err := repository.ArchiveConversationForUser(c.Request.Context(), conversationID, userID, userID)
		if errors.Is(err, dean.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
		if errors.Is(err, dean.ErrConversationArchived) {
			c.JSON(http.StatusConflict, gin.H{"error": "conversation is already archived"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not archive conversation"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"conversation": conversation})
	}
}

func handleRestoreConversation(repository *dean.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		conversationID := strings.TrimSpace(c.Param("conversationID"))
		if conversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id is required"})
			return
		}

		conversation, err := repository.RestoreConversationForUser(c.Request.Context(), conversationID, userID, userID)
		if errors.Is(err, dean.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
		if errors.Is(err, dean.ErrConversationActive) {
			c.JSON(http.StatusConflict, gin.H{"error": "conversation is already active"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not restore conversation"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"conversation": conversation})
	}
}

func handleDeleteConversation(repository *dean.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		conversationID := strings.TrimSpace(c.Param("conversationID"))
		if conversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id is required"})
			return
		}

		err := repository.DeleteConversationForUser(c.Request.Context(), conversationID, userID)
		if errors.Is(err, dean.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete conversation"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"conversation_id": conversationID,
			"deleted":         true,
		})
	}
}

func handleStreamMessage(repository *dean.Repository, streamProvider dean.AssistantStreamProvider, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := deanOfficeUserID(c)
		if !ok {
			return
		}

		conversationID := strings.TrimSpace(c.Param("conversationID"))
		messageID := strings.TrimSpace(c.Param("messageID"))
		if conversationID == "" || messageID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id and message id are required"})
			return
		}

		message, err := repository.GetAssistantMessageForUser(c.Request.Context(), conversationID, messageID, userID)
		if errors.Is(err, dean.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "assistant message not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load assistant message"})
			return
		}

		conversation, err := repository.GetConversationForUser(c.Request.Context(), conversationID, userID)
		if errors.Is(err, dean.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load conversation"})
			return
		}

		usageService, err := aiService(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not initialize ai service"})
			return
		}

		if _, err := usageService.CheckBudgetBeforeCall(c.Request.Context(), userID); err != nil {
			statusCode := http.StatusInternalServerError
			var budgetErr *ai.BudgetExceededError
			if errors.As(err, &budgetErr) {
				statusCode = http.StatusPaymentRequired
			}
			_ = repository.UpdateAssistantMessage(c.Request.Context(), dean.UpdateAssistantMessageInput{
				ConversationID: conversationID,
				MessageID:      message.ID,
				Status:         "error",
				Content:        message.Content,
				FinishReason:   "error",
				ErrorText:      err.Error(),
				UpdatedBy:      systemAccountID,
			})
			c.JSON(statusCode, gin.H{"error": err.Error()})
			return
		}

		if err := repository.UpdateAssistantMessage(c.Request.Context(), dean.UpdateAssistantMessageInput{
			ConversationID: conversationID,
			MessageID:      message.ID,
			Status:         "streaming",
			Content:        message.Content,
			FinishReason:   "",
			ErrorText:      "",
			UpdatedBy:      systemAccountID,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not begin assistant streaming"})
			return
		}

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming is not supported"})
			return
		}
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Content-Encoding", "identity")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		var writeMu sync.Mutex
		writeStreamEvent := func(event string, payload interface{}) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return writeSSEEvent(c, flusher, event, payload)
		}

		if _, err := fmt.Fprintf(c.Writer, ": %s\n\n", strings.Repeat(" ", 2048)); err == nil {
			flusher.Flush()
		}

		finalContent := message.Content
		finalFinishReason := ""
		finalUsage := dean.AssistantStreamUsage{}
		hasUsage := false
		lastPersistAt := time.Now()
		lastPersistLen := len(finalContent)
		var firstVisibleDelta atomic.Bool
		heartbeatDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(350 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-heartbeatDone:
					return
				case <-c.Request.Context().Done():
					return
				case <-ticker.C:
					if firstVisibleDelta.Load() {
						continue
					}
					_ = writeStreamEvent("message.heartbeat", dean.AssistantStreamEvent{
						Type: "message.heartbeat",
					})
				}
			}
		}()
		defer close(heartbeatDone)

		contextMessages, err := repository.ListRecentMessagesForConversation(c.Request.Context(), conversationID, 20)
		if err != nil {
			_ = repository.UpdateAssistantMessage(c.Request.Context(), dean.UpdateAssistantMessageInput{
				ConversationID: conversationID,
				MessageID:      message.ID,
				Status:         "error",
				Content:        finalContent,
				FinishReason:   "error",
				ErrorText:      "could not load conversation context",
				UpdatedBy:      systemAccountID,
			})
			_ = writeStreamEvent("message.error", dean.AssistantStreamEvent{
				Type:  "message.error",
				Error: "could not load conversation context",
			})
			return
		}

		streamErr := streamProvider.Stream(c.Request.Context(), dean.AssistantStreamRequest{
			UserID:             userID,
			ConversationID:     conversationID,
			AssistantMessageID: message.ID,
			Model:              firstNonEmpty(message.Model, "grok-3-mini"),
			Messages:           buildCommunicationPromptMessages(conversation, contextMessages),
			MaxTokens:          1024,
		}, func(event dean.AssistantStreamEvent) error {
			if event.Content != "" {
				finalContent = event.Content
			}
			if event.Delta != "" && event.Content == "" {
				finalContent += event.Delta
			}
			if event.FinishReason != "" {
				finalFinishReason = event.FinishReason
			}
			if event.Usage != nil {
				finalUsage = *event.Usage
				hasUsage = true
			}
			if event.Type == "message.delta" && event.Delta != "" {
				firstVisibleDelta.Store(true)
			}
			if err := writeStreamEvent(event.Type, event); err != nil {
				return err
			}

			if event.Type != "message.delta" {
				return nil
			}
			shouldPersist := time.Since(lastPersistAt) >= 500*time.Millisecond || (len(finalContent)-lastPersistLen) >= 128
			if !shouldPersist {
				return nil
			}
			if err := repository.UpdateAssistantMessage(c.Request.Context(), dean.UpdateAssistantMessageInput{
				ConversationID: conversationID,
				MessageID:      message.ID,
				Status:         "streaming",
				Content:        finalContent,
				FinishReason:   "",
				ErrorText:      "",
				UpdatedBy:      systemAccountID,
			}); err != nil {
				return fmt.Errorf("could not persist assistant stream progress: %w", err)
			}
			lastPersistAt = time.Now()
			lastPersistLen = len(finalContent)
			return nil
		})
		if streamErr != nil {
			status := "error"
			finishReason := "error"
			errorText := streamErr.Error()
			if isDeanStreamAbortError(streamErr) {
				status = "cancelled"
				finishReason = "cancelled"
				errorText = ""
			}
			_ = repository.UpdateAssistantMessage(c.Request.Context(), dean.UpdateAssistantMessageInput{
				ConversationID: conversationID,
				MessageID:      message.ID,
				Status:         status,
				Content:        finalContent,
				FinishReason:   finishReason,
				ErrorText:      errorText,
				UpdatedBy:      systemAccountID,
			})
			if status == "error" {
				_ = writeStreamEvent("message.error", dean.AssistantStreamEvent{
					Type:  "message.error",
					Error: streamErr.Error(),
				})
			}
			return
		}

		if finalFinishReason == "" {
			finalFinishReason = "completed"
		}
		if err := repository.UpdateAssistantMessage(c.Request.Context(), dean.UpdateAssistantMessageInput{
			ConversationID: conversationID,
			MessageID:      message.ID,
			Status:         "completed",
			Content:        finalContent,
			FinishReason:   finalFinishReason,
			ErrorText:      "",
			UpdatedBy:      systemAccountID,
		}); err != nil {
			_ = writeStreamEvent("message.error", dean.AssistantStreamEvent{
				Type:  "message.error",
				Error: "could not persist assistant completion",
			})
			return
		}

		if hasUsage {
			if err := usageService.RecordUsage(c.Request.Context(), ai.RecordUsageInput{
				UserID:         userID,
				Provider:       firstNonEmpty(message.Provider, "xai"),
				Model:          firstNonEmpty(finalUsage.Model, message.Model, "unknown"),
				RequestScope:   "communication_chat",
				TotalTokens:    finalUsage.TotalTokens,
				CostInUsdTicks: finalUsage.CostInUsdTicks,
				RequestID:      finalUsage.RequestID,
			}); err != nil {
				// Usage persistence should not retroactively flip a completed assistant response into error state.
				// The streamed assistant message has already been finalized and persisted above.
				return
			}
		}
	}
}

func buildCommunicationPromptMessages(conversation dean.Conversation, messages []dean.Message) []ai.ChatCompletionMessage {
	out := make([]ai.ChatCompletionMessage, 0, len(messages)+1)
	systemPrompt := "You are a helpful school communication assistant. Be practical and concise."
	if conversation.RecipientKind == "dean_office" {
		systemPrompt = "You are Dean Augustus Taskford (dean.taskford), the dean of this online school. Be helpful, practical, and concise."
	}
	out = append(out, ai.ChatCompletionMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}

		switch message.Role {
		case "system":
			out = append(out, ai.ChatCompletionMessage{Role: "system", Content: content})
		case "user":
			out = append(out, ai.ChatCompletionMessage{Role: "user", Content: content})
		case "assistant":
			out = append(out, ai.ChatCompletionMessage{Role: "assistant", Content: content})
		case "tool":
			out = append(out, ai.ChatCompletionMessage{Role: "assistant", Content: content})
		}
	}

	return out
}

func isDeanStreamAbortError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "broken pipe") ||
		strings.Contains(lowered, "connection reset by peer") ||
		strings.Contains(lowered, "client disconnected")
}

func shouldGenerateConversationSubject(title string) bool {
	normalized := strings.ToLower(strings.TrimSpace(title))
	return normalized == "" || normalized == "untitled conversation"
}

func buildSubjectGenerationPromptMessages(conversation dean.Conversation, messages []dean.Message) []ai.ChatCompletionMessage {
	parts := make([]string, 0, len(messages)+1)
	parts = append(parts, fmt.Sprintf("Recipient: %s", firstNonEmpty(conversation.RecipientKind, "dean_office")))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		content = truncateByRuneCount(content, 320)
		role := "Assistant"
		if message.Role == "user" {
			role = "Student"
		}
		parts = append(parts, fmt.Sprintf("%s: %s", role, content))
	}
	if len(parts) == 1 {
		parts = append(parts, "Student: General inquiry")
	}
	return []ai.ChatCompletionMessage{
		{
			Role: "user",
			Content: "Generate a concise mail subject for this conversation transcript:\n" +
				truncateByRuneCount(strings.Join(parts, "\n"), 4200),
		},
	}
}

func loadUserLanguage(ctx context.Context, db *sql.DB, userID string) string {
	var language string
	if err := db.QueryRowContext(ctx, "SELECT language FROM users WHERE id = ?", userID).Scan(&language); err != nil {
		return "en"
	}
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return "en"
	}
	return language
}

func truncateByRuneCount(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func deanOfficeUserID(c *gin.Context) (string, bool) {
	userID := strings.TrimSpace(c.GetString(contextKeyUserID))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return "", false
	}
	if userID == systemAccountID {
		c.JSON(http.StatusForbidden, gin.H{"error": "system account cannot access dean office conversations"})
		return "", false
	}
	return userID, true
}

func parseBoundedIntQuery(c *gin.Context, key string, fallback, min, max int) (int, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s must be an integer", key)})
		return 0, false
	}
	if parsed < min || parsed > max {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s must be between %d and %d", key, min, max)})
		return 0, false
	}
	return parsed, true
}

func writeSSEEvent(c *gin.Context, flusher http.Flusher, event string, payload interface{}) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, bytes); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
