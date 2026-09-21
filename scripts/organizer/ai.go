package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	hasAttachment := capture.AttachmentPath != ""
	systemPrompt := `你是 Faber 的个人日程与项目整理助手。把用户输入转换为结构化事项。只输出 JSON 对象，格式为 {"items":[...]}。每个事项字段：type(task|event|reminder|note)、title、description、startAt、endAt、dueAt、reminderAt、timezone、allDay、recurrenceRule、priority(0-3)、certainty(confirmed|tentative)、durationMinutes、availableFrom、availableUntil、project、tags、location、people、confidence(0-1)、ambiguities。时间使用 RFC3339 并带时区。startAt 表示实际开始或发生时间，dueAt 表示最晚完成时间，reminderAt 表示通知时间；不要混淆。没有日期时保留为空，模糊信息写入 ambiguities，不要伪造。一个输入可以拆成多个事项。默认时区为 ` + a.zone + `。当前时间为 ` + now + `。`
	if hasAttachment {
		systemPrompt = `你是 Faber 的截图事项整理助手。阅读截图，只提取需要行动、安排、跟进或保留的重要信息，不要输出无关的界面文字或完整 OCR。只输出 JSON：{"items":[...]}。每项可用字段：type(task|event|reminder|note)、title、description、startAt、endAt、dueAt、reminderAt、timezone、allDay、priority(0-3)、certainty(confirmed|tentative)、durationMinutes、project、tags、location、people、confidence(0-1)、ambiguities。时间用 RFC3339；无法确认的日期、对象或行动写入 ambiguities，绝不猜测。默认时区 ` + a.zone + `，当前时间 ` + now + `。`
	}
	memories := ""
	if len(memoryContext) > 0 {
		memories = memoryContext[0]
	}
	if strings.TrimSpace(memories) != "" {
		systemPrompt += "以下是用户已明确批准的个人规则，只在与当前输入相关时使用；不得自行修改：\n" + memories
	}

	var userContent any = "请整理以下内容：\n" + capture.RawText
	if hasAttachment {
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
	maxTokens := 2200
	payload := map[string]any{
		"model": a.model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"response_format":       map[string]string{"type": "json_object"},
		"temperature":           0.1,
		"max_completion_tokens": maxTokens,
	}
	if hasAttachment {
		maxTokens = 800
		payload["max_completion_tokens"] = maxTokens
		payload["reasoning_effort"] = "low"
	}
	content, status, err := a.request(ctx, payload)
	if err != nil && status == http.StatusBadRequest {
		delete(payload, "reasoning_effort")
		delete(payload, "max_completion_tokens")
		payload["max_tokens"] = maxTokens
		content, status, err = a.request(ctx, payload)
	}
	if err != nil {
		errorClass, reason := classifyAIError(err, status, hasAttachment)
		log.Printf("AI parse fallback capture_id=%s attachment=%t status=%d error_class=%s", capture.ID, hasAttachment, status, errorClass)
		return fallbackParse(capture, a.zone, reason), nil
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

func classifyAIError(err error, status int, hasAttachment bool) (string, string) {
	prefix := "原文已保存。"
	if hasAttachment {
		prefix = "截图已保存。"
	}
	switch {
	case status == http.StatusBadRequest && hasAttachment:
		return "visual_request_rejected", prefix + "当前模型或中转站拒绝了图片识别请求，请检查模型能力后重试"
	case status == http.StatusBadRequest:
		return "request_rejected", prefix + "当前模型或中转站拒绝了整理请求，请检查模型配置后重试"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "authentication", prefix + "服务器的 AI 鉴权或密钥配置有误，请检查后重试"
	case status == http.StatusRequestEntityTooLarge:
		return "image_too_large", prefix + "图片超过中转站允许的大小，请压缩截图后重试"
	case status == http.StatusTooManyRequests:
		return "rate_limited", prefix + "AI 上游当前限流，请稍后重新识别"
	case status >= 500:
		return "upstream_unavailable", prefix + "AI 上游暂时不可用，请稍后重试"
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return "timeout", prefix + "AI 识别超过等待时间，请重新识别"
	}
	if errors.Is(err, context.Canceled) {
		return "request_cancelled", prefix + "本次 AI 请求已中断，请重试"
	}
	return "network", prefix + "AI 网络请求失败，请检查中转站连接后重试"
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
