package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	return &AIClient{baseURL: cfg.AIBaseURL, apiKey: cfg.AIAPIKey, model: cfg.AIModel, zone: cfg.Timezone, client: &http.Client{Transport: transport, Timeout: 60 * time.Second}}
}

func fallbackParse(capture Capture, zone, reason string) ParseResult {
	title := strings.TrimSpace(capture.RawText)
	if newline := strings.IndexByte(title, '\n'); newline >= 0 {
		title = strings.TrimSpace(title[:newline])
	}
	runes := []rune(title)
	if len(runes) > 200 {
		title = string(runes[:200]) + "…"
	}
	if title == "" {
		title = "查看截图并整理事项"
	}
	return ParseResult{Items: []Candidate{{
		Type: "task", Title: title, Timezone: zone, Confidence: 0.2,
		Ambiguities: []string{reason},
	}}}
}

func (a *AIClient) Parse(ctx context.Context, capture Capture, memoryContext ...string) (ParseResult, error) {
	if strings.TrimSpace(capture.RawText) == "" && capture.AttachmentPath == "" {
		return ParseResult{}, errors.New("capture has no text or attachment")
	}
	if a.apiKey == "" {
		return fallbackParse(capture, a.zone, "AI 尚未配置，请手动检查标题、日期和提醒时间"), nil
	}

	location, _ := time.LoadLocation(a.zone)
	now := time.Now().In(location).Format(time.RFC3339)
	systemPrompt := `你是 Faber 的个人日程与项目整理助手。把用户输入或截图转换为结构化事项。只输出 JSON 对象，格式为 {"items":[...]}。每个事项字段：type(task|event|reminder|note)、title、description、startAt、endAt、dueAt、reminderAt、timezone、allDay、recurrenceRule、priority(0-3)、certainty(confirmed|tentative)、durationMinutes、availableFrom、availableUntil、project、tags、location、people、confidence(0-1)、ambiguities。时间使用 RFC3339 并带时区。startAt 表示实际开始或发生时间，dueAt 表示最晚完成时间，reminderAt 表示发送通知的时间；三者不可混淆。用户说“提醒我”时必须给出 reminderAt，优先使用明确的提醒时间，否则使用 startAt 或 dueAt。没有日期时保留为空。模糊日期存在多种解释时写入 ambiguities，不要伪造。明确是临时、可能、暂定的安排使用 certainty=tentative。一个输入可以拆成多个事项。默认时区为 ` + a.zone + `。当前时间为 ` + now + `。`
	memories := ""
	if len(memoryContext) > 0 {
		memories = memoryContext[0]
	}
	if strings.TrimSpace(memories) != "" {
		systemPrompt += "以下是用户已明确批准的个人规则，只在与当前输入相关时使用；不得自行修改：\n" + memories
	}

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
		return fallbackParse(capture, a.zone, "AI 服务暂时不可用，已保留原文；请手动检查标题、日期和提醒时间"), nil
	}
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var result ParseResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err != nil {
		return fallbackParse(capture, a.zone, "AI 返回格式异常，已保留原文；请手动检查标题、日期和提醒时间"), nil
	}
	if len(result.Items) == 0 {
		return fallbackParse(capture, a.zone, "AI 没有识别出事项，已保留原文；请手动检查标题、日期和提醒时间"), nil
	}
	for index := range result.Items {
		result.Items[index] = normalizeCandidate(result.Items[index], a.zone)
		if result.Items[index].Title == "" {
			return fallbackParse(capture, a.zone, "AI 返回内容不完整，已保留原文；请手动检查标题、日期和提醒时间"), nil
		}
	}
	applyCaptureInvariants(capture, &result, a.zone)
	return result, nil
}

func applyCaptureInvariants(capture Capture, result *ParseResult, zone string) {
	text := capture.RawText
	periodDefaults := map[string]int{"早上": 9, "上午": 9, "中午": 12, "下午": 15, "傍晚": 18, "晚上": 20, "下班后": 18}
	defaultHour := -1
	for period, hour := range periodDefaults {
		if strings.Contains(text, period) {
			defaultHour = hour
			break
		}
	}
	location, _ := time.LoadLocation(zone)
	for index := range result.Items {
		item := &result.Items[index]
		if defaultHour >= 0 && item.StartAt != "" && location != nil {
			if parsed, err := time.Parse(time.RFC3339, item.StartAt); err == nil {
				local := parsed.In(location)
				if local.Hour() == 0 && local.Minute() == 0 {
					item.StartAt = time.Date(local.Year(), local.Month(), local.Day(), defaultHour, 0, 0, 0, location).UTC().Format(time.RFC3339)
					item.Ambiguities = append(item.Ambiguities, "使用系统模糊时间默认值，请确认具体时间")
				}
			}
		}
		if strings.Contains(text, "提醒") && item.ReminderAt == "" {
			if item.StartAt != "" {
				item.ReminderAt = item.StartAt
			} else if item.DueAt != "" {
				item.ReminderAt = item.DueAt
			} else {
				item.Ambiguities = append(item.Ambiguities, "用户要求提醒，但尚未提供可用的提醒日期")
			}
		}
	}
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
