package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamChatCompletion_EmitsDeltaDoneAndUsage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["stream"] != true {
			t.Fatalf("stream must be true")
		}
		options, ok := payload["stream_options"].(map[string]interface{})
		if !ok || options["include_usage"] != true {
			t.Fatalf("stream_options.include_usage must be true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"req_1\",\"model\":\"grok-3-mini\",\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":\"\"}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"req_1\",\"model\":\"grok-3-mini\",\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"req_1\",\"model\":\"grok-3-mini\",\"choices\":[],\"usage\":{\"total_tokens\":42,\"cost_in_usd_ticks\":99}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewXAIClient()
	events := make([]StreamChatCompletionEvent, 0)
	usage, err := client.StreamChatCompletion(context.Background(), StreamChatCompletionRequest{
		BaseURL:  server.URL,
		Token:    "test-token",
		Model:    "grok-3-mini",
		Messages: []ChatCompletionMessage{{Role: "user", Content: "Hi"}},
	}, func(event StreamChatCompletionEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChatCompletion returned error: %v", err)
	}

	if usage.TotalTokens != 42 || usage.CostInUsdTicks != 99 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	if usage.Model != "grok-3-mini" || usage.RequestID != "req_1" {
		t.Fatalf("unexpected metadata: %+v", usage)
	}
	if len(events) != 4 {
		t.Fatalf("unexpected events count: %d", len(events))
	}
	if events[0].Type != StreamEventDelta || events[0].Delta != "Hello" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Type != StreamEventDelta || events[1].Delta != " world" {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if events[2].Type != StreamEventDone || events[2].FinishReason != "stop" {
		t.Fatalf("unexpected done event: %+v", events[2])
	}
	if events[3].Type != StreamEventUsage || events[3].Usage.TotalTokens != 42 {
		t.Fatalf("unexpected usage event: %+v", events[3])
	}
}

func TestStreamChatCompletion_MalformedChunk(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {invalid json}\n\n")
	}))
	defer server.Close()

	client := NewXAIClient()
	_, err := client.StreamChatCompletion(context.Background(), StreamChatCompletionRequest{
		BaseURL:  server.URL,
		Token:    "test-token",
		Model:    "grok-3-mini",
		Messages: []ChatCompletionMessage{{Role: "user", Content: "Hi"}},
	}, func(StreamChatCompletionEvent) error { return nil })
	if err == nil {
		t.Fatalf("expected error for malformed chunk")
	}
}

func TestStreamChatCompletion_UpstreamHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewXAIClient()
	_, err := client.StreamChatCompletion(context.Background(), StreamChatCompletionRequest{
		BaseURL:  server.URL,
		Token:    "test-token",
		Model:    "grok-3-mini",
		Messages: []ChatCompletionMessage{{Role: "user", Content: "Hi"}},
	}, func(StreamChatCompletionEvent) error { return nil })
	if err == nil {
		t.Fatalf("expected upstream HTTP error")
	}
}
