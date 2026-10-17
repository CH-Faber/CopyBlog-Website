package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAIParseFallsBackWhenUpstreamFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	client := &AIClient{
		baseURL: upstream.URL,
		apiKey:  "test-key",
		model:   "test-model",
		zone:    "Asia/Shanghai",
		client:  upstream.Client(),
	}
	result, err := client.Parse(context.Background(), Capture{RawText: "检查事项系统\n后续说明"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "检查事项系统" {
		t.Fatalf("unexpected fallback: %#v", result)
	}
	if len(result.Items[0].Ambiguities) != 1 || !strings.Contains(result.Items[0].Ambiguities[0], "AI 上游暂时不可用") {
		t.Fatalf("missing fallback explanation: %#v", result.Items[0].Ambiguities)
	}
}

func TestAIParseUsesOptimizedImageRequest(t *testing.T) {
	imagePath := t.TempDir() + "/capture.png"
	if err := os.WriteFile(imagePath, []byte("fake image bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["reasoning_effort"] != "low" {
			t.Fatalf("unexpected reasoning effort: %#v", payload["reasoning_effort"])
		}
		if payload["max_completion_tokens"] != float64(800) {
			t.Fatalf("unexpected image token budget: %#v", payload["max_completion_tokens"])
		}
		messages, ok := payload["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Fatalf("missing messages: %#v", payload["messages"])
		}
		user, ok := messages[1].(map[string]any)
		content, contentOK := user["content"].([]any)
		if !ok || !contentOK || len(content) != 2 {
			t.Fatalf("image content was not sent: %#v", messages[1])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[{\"type\":\"task\",\"title\":\"同步截图事项\",\"priority\":2,\"confidence\":0.9}]}"}}]}`))
	}))
	defer upstream.Close()

	client := &AIClient{baseURL: upstream.URL, apiKey: "test-key", model: "test-model", zone: "Asia/Shanghai", client: upstream.Client()}
	result, err := client.Parse(context.Background(), Capture{ID: "capture-image", RawText: "帮我同步一下", AttachmentPath: imagePath, AttachmentMime: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "同步截图事项" || result.Items[0].Priority != 2 {
		t.Fatalf("unexpected image parse result: %#v", result)
	}
}

func TestAIParseSendsAllCaptureImagesTogether(t *testing.T) {
	dir := t.TempDir()
	firstPath := dir + "/first.png"
	secondPath := dir + "/second.jpg"
	if err := os.WriteFile(firstPath, []byte("first image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("second image"), 0o600); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		messages := payload["messages"].([]any)
		user := messages[1].(map[string]any)
		content := user["content"].([]any)
		if len(content) != 3 {
			t.Fatalf("content parts = %d; want text plus two images: %#v", len(content), content)
		}
		for index, part := range content[1:] {
			value := part.(map[string]any)
			if value["type"] != "image_url" {
				t.Fatalf("content part %d is not an image: %#v", index+1, value)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[{\"type\":\"task\",\"title\":\"合并识别\"}]}"}}]}`))
	}))
	defer upstream.Close()

	client := &AIClient{baseURL: upstream.URL, apiKey: "test-key", model: "test-model", zone: "Asia/Shanghai", client: upstream.Client()}
	result, err := client.Parse(context.Background(), Capture{Attachments: []CaptureAttachment{{Name: "first.png", Mime: "image/png", Path: firstPath}, {Name: "second.jpg", Mime: "image/jpeg", Path: secondPath}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "合并识别" {
		t.Fatalf("unexpected multi-image parse result: %#v", result)
	}
}

func TestFallbackTitleIsBounded(t *testing.T) {
	result := fallbackParse(Capture{RawText: strings.Repeat("事项", 150)}, "Asia/Shanghai", "test")
	if length := len([]rune(result.Items[0].Title)); length > 201 {
		t.Fatalf("fallback title too long: %d", length)
	}
}

func TestReminderInvariantAndFuzzyTimeDefault(t *testing.T) {
	result := ParseResult{Items: []Candidate{{Type: "reminder", Title: "报名四级", StartAt: "2026-09-24T00:00:00+08:00"}}}
	applyCaptureInvariants(Capture{RawText: "提醒我大后天早上报名四级"}, &result, "Asia/Shanghai")
	item := result.Items[0]
	if item.StartAt != "2026-09-24T01:00:00Z" {
		t.Fatalf("fuzzy morning was not resolved: %s", item.StartAt)
	}
	if item.ReminderAt != item.StartAt {
		t.Fatalf("reminder intent did not create a reminder: %#v", item)
	}
}
