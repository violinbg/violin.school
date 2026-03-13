package dean

import (
	"context"
	"errors"

	"github.com/violinbg/violin.school/internal/ai"
)

type AssistantStreamRequest struct {
	UserID             string
	ConversationID     string
	AssistantMessageID string
	Model              string
	Messages           []ai.ChatCompletionMessage
	MaxTokens          int
}

type AssistantStreamEvent struct {
	Type         string                `json:"type"`
	Delta        string                `json:"delta,omitempty"`
	Content      string                `json:"content,omitempty"`
	FinishReason string                `json:"finish_reason,omitempty"`
	Error        string                `json:"error,omitempty"`
	Usage        *AssistantStreamUsage `json:"usage,omitempty"`
}

type AssistantStreamUsage struct {
	TotalTokens    int64  `json:"total_tokens"`
	CostInUsdTicks int64  `json:"cost_in_usd_ticks"`
	Model          string `json:"model,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
}

type AssistantStreamProvider interface {
	Stream(ctx context.Context, request AssistantStreamRequest, emit func(AssistantStreamEvent) error) error
}

type XAIConfigResolver func(ctx context.Context) (baseURL string, token string, err error)

type XAIStreamProvider struct {
	client        *ai.XAIClient
	resolveConfig XAIConfigResolver
}

func NewXAIStreamProvider(client *ai.XAIClient, resolveConfig XAIConfigResolver) XAIStreamProvider {
	return XAIStreamProvider{
		client:        client,
		resolveConfig: resolveConfig,
	}
}

func (p XAIStreamProvider) Stream(ctx context.Context, request AssistantStreamRequest, emit func(AssistantStreamEvent) error) error {
	if p.client == nil {
		return errors.New("xai stream provider client is not configured")
	}
	if p.resolveConfig == nil {
		return errors.New("xai stream provider config resolver is not configured")
	}

	baseURL, token, err := p.resolveConfig(ctx)
	if err != nil {
		return err
	}

	if err := emit(AssistantStreamEvent{Type: "message.start"}); err != nil {
		return err
	}

	_, err = p.client.StreamChatCompletion(ctx, ai.StreamChatCompletionRequest{
		BaseURL:   baseURL,
		Token:     token,
		Model:     request.Model,
		Messages:  request.Messages,
		MaxTokens: request.MaxTokens,
	}, func(event ai.StreamChatCompletionEvent) error {
		switch event.Type {
		case ai.StreamEventDelta:
			return emit(AssistantStreamEvent{
				Type:  "message.delta",
				Delta: event.Delta,
			})
		case ai.StreamEventDone:
			return emit(AssistantStreamEvent{
				Type:         "message.done",
				FinishReason: event.FinishReason,
			})
		case ai.StreamEventUsage:
			return emit(AssistantStreamEvent{
				Type: "message.usage",
				Usage: &AssistantStreamUsage{
					TotalTokens:    event.Usage.TotalTokens,
					CostInUsdTicks: event.Usage.CostInUsdTicks,
					Model:          event.Usage.Model,
					RequestID:      event.Usage.RequestID,
				},
			})
		default:
			return nil
		}
	})
	return err
}
