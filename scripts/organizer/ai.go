package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type AIClient struct {
	baseURL string
	apiKey  string
	model   string
	zone    string
	client  *http.Client
}

func newAIClient(cfg Config) *AIClient {
	return &AIClient{baseURL: cfg.AIBaseURL, apiKey: cfg.AIAPIKey, model: cfg.AIModel, zone: cfg.Timezone, client: &http.Client{Timeout: 60 * time.Second}}
}

func (a *AIClient) Parse(ctx context.Context, capture Capture) (ParseResult, error) {
	if strings.TrimSpace(capture.RawText) == "" && capture.AttachmentPath == "" {
		return ParseResult{}, errors.New("capture has no text or attachment")
	}
	if a.apiKey == "" {
		title := strings.TrimSpace(capture.RawText)
		if title == "" {
			title = "查看截图并整理事项"
		}
		return ParseResult{Items: []Candidate{{Type: "task", Title: title, Timezone: a.zone, Confidence: 0.2, Ambiguities: []string{"AI 尚未配置，请手动检查"}}}}, nil
	}

	location, _ := time.LoadLocation(a.zone)
	now := time.Now().In(location).Format(time.RFC3339)
	systemPrompt := `你是 Faber 的个人事项整理助手。把用户输入或截图转换为结构化事项。只输出 JSON 对象，格式为 {"items":[...]}。每个事项字段：type(task|event|reminder)、title、description、startAt、endAt、dueAt、reminderAt、timezone、allDay、recurrenceRule、priority(0-3)、project、tags、location、people、confidence(0-1)、ambiguities。时间使用 RFC3339 并带时区。不要凭空确定模糊时间；如必须暂定，在 ambiguities 中明确说明。没有日期时保留为空。一个输入可以拆成多个事项。默认时区为 ` + a.zone + `。当前时间为 ` + now + `。`

	var userContent any = "请整理以下内容：\n" + capture.RawText
	if capture.AttachmentPath != "" {
		data, err := os.ReadFile(capture.AttachmentPath)
		if err != nil {
			return ParseResult{}, err
		}
		mime := capture.AttachmentMime
		if mime == "" {
			mime = "image/png"
		}
		userContent = []map[string]any{
			{"type": "text", "text": "请结合截图整理事项。用户补充文字：\n" + capture.RawText},
			{"type": "image_url", "image_url": map[string]string{"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}},
		}
	}
	payload := map[string]any{
		"model": a.model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"response_format":       map[string]string{"type": "json_object"},
		"temperature":           0.1,
		"max_completion_tokens": 2200,
	}
	content, status, err := a.request(ctx, payload)
	if err != nil && status == http.StatusBadRequest {
		delete(payload, "max_completion_tokens")
		payload["max_tokens"] = 2200
		content, _, err = a.request(ctx, payload)
	}
	if err != nil {
		return ParseResult{}, err
	}
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var result ParseResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err != nil {
		return ParseResult{}, fmt.Errorf("decode AI result: %w", err)
	}
	if len(result.Items) == 0 {
		return ParseResult{}, errors.New("AI returned no items")
	}
	for index := range result.Items {
		result.Items[index] = normalizeCandidate(result.Items[index], a.zone)
		if result.Items[index].Title == "" {
			return ParseResult{}, errors.New("AI returned an item without title")
		}
	}
	return result, nil
}

func (a *AIClient) request(ctx context.Context, payload map[string]any) (string, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", resp.StatusCode, fmt.Errorf("AI upstream returned HTTP %d", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil || len(decoded.Choices) == 0 {
		return "", resp.StatusCode, errors.New("AI upstream returned an invalid response")
	}
	return decoded.Choices[0].Message.Content, resp.StatusCode, nil
}
