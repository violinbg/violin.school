package ai

import (
	"bytes"
	"context"
	"encoding/json"
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

func NewXAIClient() *XAIClient {
	return &XAIClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *XAIClient) TestToken(ctx context.Context, baseURL, token string) error {
	_, err := c.TestTokenWithUsage(ctx, baseURL, token)
	return err
}

func (c *XAIClient) TestTokenWithUsage(ctx context.Context, baseURL, token string) (UsageMetrics, error) {
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
