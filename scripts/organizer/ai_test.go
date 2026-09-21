package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	if len(result.Items[0].Ambiguities) != 1 || !strings.Contains(result.Items[0].Ambiguities[0], "AI 服务暂时不可用") {
		t.Fatalf("missing fallback explanation: %#v", result.Items[0].Ambiguities)
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
