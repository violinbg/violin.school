package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type XAIClient struct {
	httpClient *http.Client
}

type UsageMetrics struct {
	TotalTokens    int64
	CostInUsdTicks int64
	Model          string
	RequestID      string
}

type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamChatCompletionRequest struct {
	BaseURL   string
	Token     string
	Model     string
	Messages  []ChatCompletionMessage
	MaxTokens int
}

type StreamChatCompletionEvent struct {
	Type         string
	Delta        string
	FinishReason string
	Usage        UsageMetrics
}

const (
	StreamEventDelta = "delta"
	StreamEventDone  = "done"
	StreamEventUsage = "usage"
)

const (
	defaultRequestTimeout = 15 * time.Second
	defaultStreamTimeout  = 2 * time.Minute
)

func NewXAIClient() *XAIClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	return &XAIClient{
		httpClient: &http.Client{Transport: transport},
	}
}

func (c *XAIClient) TestToken(ctx context.Context, baseURL, token string) error {
	_, err := c.TestTokenWithUsage(ctx, baseURL, token)
	return err
}

func (c *XAIClient) TestTokenWithUsage(ctx context.Context, baseURL, token string) (UsageMetrics, error) {
	ctx, cancel := withDefaultTimeout(ctx, defaultRequestTimeout)
	defer cancel()

	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return UsageMetrics{}, fmt.Errorf("xai base url is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return UsageMetrics{}, fmt.Errorf("invalid xai base url: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return UsageMetrics{}, fmt.Errorf("xai token is empty")
	}

	payload := map[string]interface{}{
		"model": "grok-3-mini",
		"messages": []map[string]string{
			{"role": "system", "content": "Health check"},
			{"role": "user", "content": "Return pong"},
		},
		"max_tokens": 8,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return UsageMetrics{}, fmt.Errorf("encode test payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return UsageMetrics{}, fmt.Errorf("create test request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return UsageMetrics{}, fmt.Errorf("xai request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return UsageMetrics{}, fmt.Errorf("xai test failed: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			TotalTokens    int64 `json:"total_tokens"`
			CostInUsdTicks int64 `json:"cost_in_usd_ticks"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return UsageMetrics{}, fmt.Errorf("parse xai test response: %w", err)
	}

	return UsageMetrics{
		TotalTokens:    parsed.Usage.TotalTokens,
		CostInUsdTicks: parsed.Usage.CostInUsdTicks,
		Model:          parsed.Model,
		RequestID:      parsed.ID,
	}, nil
}

func (c *XAIClient) StreamChatCompletion(ctx context.Context, request StreamChatCompletionRequest, emit func(StreamChatCompletionEvent) error) (UsageMetrics, error) {
	ctx, cancel := withDefaultTimeout(ctx, defaultStreamTimeout)
	defer cancel()

	if emit == nil {
		return UsageMetrics{}, errors.New("emit callback is required")
	}
	baseURL := strings.TrimSuffix(strings.TrimSpace(request.BaseURL), "/")
	if baseURL == "" {
		return UsageMetrics{}, fmt.Errorf("xai base url is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return UsageMetrics{}, fmt.Errorf("invalid xai base url: %w", err)
	}
	if strings.TrimSpace(request.Token) == "" {
		return UsageMetrics{}, fmt.Errorf("xai token is empty")
	}
	if strings.TrimSpace(request.Model) == "" {
		return UsageMetrics{}, fmt.Errorf("xai model is required")
	}
	if len(request.Messages) == 0 {
		return UsageMetrics{}, fmt.Errorf("at least one chat message is required")
	}

	payload := map[string]interface{}{
		"model":    request.Model,
		"messages": request.Messages,
		"stream":   true,
		"stream_options": map[string]bool{
			"include_usage": true,
		},
	}
	if request.MaxTokens > 0 {
		payload["max_tokens"] = request.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return UsageMetrics{}, fmt.Errorf("encode stream payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return UsageMetrics{}, fmt.Errorf("create stream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+request.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return UsageMetrics{}, fmt.Errorf("xai stream request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return UsageMetrics{}, fmt.Errorf("xai stream failed: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	usage := UsageMetrics{}
	doneEmitted := false
	parseErr := consumeSSEData(res.Body, func(data string) error {
		trimmed := strings.TrimSpace(data)
		if trimmed == "" {
			return nil
		}
		if trimmed == "[DONE]" {
			if !doneEmitted {
				if err := emit(StreamChatCompletionEvent{Type: StreamEventDone}); err != nil {
					return err
				}
				doneEmitted = true
			}
			return nil
		}

		var chunk xaiStreamChunk
		if err := json.Unmarshal([]byte(trimmed), &chunk); err != nil {
			return fmt.Errorf("parse xai stream chunk: %w", err)
		}
		if chunk.Error != nil {
			if chunk.Error.Message != "" {
				return fmt.Errorf("xai stream error: %s", chunk.Error.Message)
			}
			return errors.New("xai stream returned an error chunk")
		}

		if chunk.ID != "" {
			usage.RequestID = chunk.ID
		}
		if chunk.Model != "" {
			usage.Model = chunk.Model
		}
		if chunk.Usage != nil {
			usage.TotalTokens = chunk.Usage.TotalTokens
			usage.CostInUsdTicks = chunk.Usage.CostInUsdTicks
			if err := emit(StreamChatCompletionEvent{
				Type:  StreamEventUsage,
				Usage: usage,
			}); err != nil {
				return err
			}
		}

		for _, choice := range chunk.Choices {
			if delta := choice.Delta.Content; delta != "" {
				if err := emit(StreamChatCompletionEvent{
					Type:  StreamEventDelta,
					Delta: delta,
				}); err != nil {
					return err
				}
			}
			if choice.FinishReason != "" && !doneEmitted {
				if err := emit(StreamChatCompletionEvent{
					Type:         StreamEventDone,
					FinishReason: choice.FinishReason,
				}); err != nil {
					return err
				}
				doneEmitted = true
			}
		}
		return nil
	})
	if parseErr != nil {
		return UsageMetrics{}, fmt.Errorf("consume xai stream: %w", parseErr)
	}

	return usage, nil
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func consumeSSEData(body io.Reader, handle func(data string) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if field != "data" {
			continue
		}
		payload := strings.TrimPrefix(value, " ")
		if err := handle(payload); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

type xaiStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		TotalTokens    int64 `json:"total_tokens"`
		CostInUsdTicks int64 `json:"cost_in_usd_ticks"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}
