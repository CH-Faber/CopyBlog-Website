package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	assistantProviderTimeout = 20 * time.Second
	assistantProviderBodyCap = 256 << 10
	assistantContextBodyCap  = 64 << 10
	assistantPlanningRetries = 1
)

type assistantProviderError struct {
	Class     string
	Status    int
	Retryable bool
	Cause     error
}

func (e *assistantProviderError) Error() string {
	if e == nil {
		return "assistant provider error"
	}
	return "assistant provider " + e.Class
}

func (e *assistantProviderError) Unwrap() error { return e.Cause }

type assistantConversationView struct {
	AssistantConversation
	Turns []AssistantTurn `json:"turns,omitempty"`
}

type assistantTurnRequest struct {
	Content        string `json:"content,omitempty"`
	Text           string `json:"text,omitempty"`
	Message        string `json:"message,omitempty"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

type assistantCaptureTurnRequest struct {
	CaptureID string `json:"captureId"`
}

type assistantCaptureTurnResponse struct {
	UserTurn      AssistantTurn `json:"userTurn"`
	AssistantTurn AssistantTurn `json:"assistantTurn"`
}

type assistantErrorResponse struct {
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	Class     string `json:"class,omitempty"`
	Retryable bool   `json:"retryable"`
}

type assistantPlanResponse struct {
	RequestID              string               `json:"requestId"`
	Kind                   string               `json:"kind,omitempty"`
	Reply                  string               `json:"reply,omitempty"`
	Plan                   *AssistantPlanRecord `json:"plan,omitempty"`
	Receipt                *AssistantPlanRecord `json:"receipt,omitempty"`
	ClarificationQuestions []string             `json:"clarificationQuestions,omitempty"`
}

func assistantRequestID(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if value != "" && len(value) <= 128 && !strings.ContainsAny(value, "\r\n") {
		return value
	}
	return newID("req")
}

func writeAssistantError(w http.ResponseWriter, status int, requestID, class, message string, retryable bool) {
	w.Header().Set("X-Request-ID", requestID)
	writeJSON(w, status, assistantErrorResponse{Message: message, RequestID: requestID, Class: class, Retryable: retryable})
}

func (s *Server) handleAssistantCreateConversation(w http.ResponseWriter, _ *http.Request) {
	conversation, err := s.store.createAssistantConversationWithMetadata("openai-compatible", s.cfg.AIModel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create assistant conversation")
		return
	}
	writeJSON(w, http.StatusCreated, conversation)
}

func (s *Server) handleAssistantListConversations(w http.ResponseWriter, r *http.Request) {
	conversations, err := s.store.listAssistantConversations(queryLimit(r, 100, 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list assistant conversations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": conversations})
}

func (s *Server) handleAssistantGetConversation(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.store.getAssistantConversation(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "assistant conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get assistant conversation")
		return
	}
	turns, err := s.store.listAssistantTurns(conversation.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list assistant turns")
		return
	}
	writeJSON(w, http.StatusOK, assistantConversationView{AssistantConversation: conversation, Turns: turns})
}

func (s *Server) handleAssistantDeleteConversation(w http.ResponseWriter, r *http.Request) {
	err := s.store.deleteAssistantConversation(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "assistant conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete assistant conversation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAssistantCaptureTurn(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.store.getAssistantConversation(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "assistant conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get assistant conversation")
		return
	}
	var request assistantCaptureTurnRequest
	if err := decodeJSON(r, &request, 4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.CaptureID = strings.TrimSpace(request.CaptureID)
	if request.CaptureID == "" {
		writeError(w, http.StatusBadRequest, "captureId is required")
		return
	}
	capture, err := s.store.getCapture(request.CaptureID)
	if errors.Is(err, sql.ErrNoRows) || capture.WorkspaceID != conversation.WorkspaceID {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get capture")
		return
	}
	if !capture.HasAttachment {
		writeError(w, http.StatusBadRequest, "capture does not contain an image")
		return
	}

	var parsed ParseResult
	if len(capture.AIResult) > 0 {
		if err := json.Unmarshal(capture.AIResult, &parsed); err != nil {
			writeError(w, http.StatusConflict, "capture result is not available")
			return
		}
	}
	turns, err := s.store.listAssistantTurns(conversation.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list assistant turns")
		return
	}
	var response assistantCaptureTurnResponse
	for _, turn := range turns {
		if turn.StructuredResultRef != capture.ID {
			continue
		}
		if turn.Role == "user" {
			response.UserTurn = turn
		} else if turn.Role == "assistant" {
			response.AssistantTurn = turn
		}
	}

	attachments := captureAttachmentValues(capture)
	names := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if name := strings.TrimSpace(attachment.Name); name != "" {
			names = append(names, name)
		}
	}
	label := "图片"
	if len(names) == 1 {
		label = "图片「" + names[0] + "」"
	} else if len(names) > 1 {
		label = fmt.Sprintf("%d 张图片（%s）", len(names), strings.Join(names, "、"))
	}
	if response.UserTurn.ID == "" {
		content := label
		if raw := strings.TrimSpace(capture.RawText); raw != "" {
			content += "\n" + raw
		}
		structured, _ := json.Marshal(map[string]any{"kind": "image_input", "captureId": capture.ID, "attachmentNames": names})
		response.UserTurn, err = s.store.appendAssistantTurnRecord(conversation.ID, "user", content, structured, capture.ID, "", "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "save image turn")
			return
		}
	}
	if response.AssistantTurn.ID == "" {
		content := assistantCaptureResultMessage(label, parsed.Items)
		structured, _ := json.Marshal(map[string]any{"kind": "image_capture", "captureId": capture.ID, "candidates": parsed.Items})
		response.AssistantTurn, err = s.store.appendAssistantTurn(conversation.ID, content, structured, capture.ID, "openai-compatible", s.cfg.AIModel)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "save image result turn")
			return
		}
	}
	writeJSON(w, http.StatusCreated, response)
}

func assistantCaptureResultMessage(label string, items []Candidate) string {
	if len(items) == 0 {
		return "已收到" + label + "，但没有识别出明确事项。原图已保存到待整理。"
	}
	titles := make([]string, 0, len(items))
	for _, item := range items {
		if title := strings.TrimSpace(item.Title); title != "" {
			titles = append(titles, title)
		}
		if len(titles) == 8 {
			break
		}
	}
	message := fmt.Sprintf("已识别%s，发现 %d 个事项，已放入待整理。", label, len(items))
	if len(titles) > 0 {
		message += "\n" + strings.Join(titles, "；")
	}
	return message
}

func (s *Server) handleAssistantTurn(w http.ResponseWriter, r *http.Request) {
	requestID := assistantRequestID(r)
	w.Header().Set("X-Request-ID", requestID)
	conversationID := r.PathValue("id")
	conversation, err := s.store.getAssistantConversation(conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		writeAssistantError(w, http.StatusNotFound, requestID, "not_found", "assistant conversation not found", false)
		return
	}
	if err != nil {
		writeAssistantError(w, http.StatusInternalServerError, requestID, "storage", "get assistant conversation", false)
		return
	}
	var request assistantTurnRequest
	if err := decodeJSON(r, &request, maxAssistantTurnText+4096); err != nil {
		writeAssistantError(w, http.StatusBadRequest, requestID, "invalid_request", err.Error(), false)
		return
	}
	content := strings.TrimSpace(request.Content)
	if content == "" {
		content = strings.TrimSpace(request.Text)
	}
	if content == "" {
		content = strings.TrimSpace(request.Message)
	}
	if content == "" {
		writeAssistantError(w, http.StatusBadRequest, requestID, "invalid_request", "content is required", false)
		return
	}
	sourceRequestHash := assistantSourceRequestHash(content)
	if key := strings.TrimSpace(request.IdempotencyKey); key != "" {
		if existing, lookupErr := s.store.getAssistantPlanByIdempotency(conversation.WorkspaceID, key); lookupErr == nil {
			if existing.ConversationID != conversation.ID || existing.SourceRequestHash != sourceRequestHash {
				writeAssistantError(w, http.StatusConflict, requestID, "idempotency_conflict", "idempotency key already used for a different request", false)
				return
			}
			response := assistantPlanResponse{RequestID: requestID, Plan: &existing}
			if existing.State == AssistantPlanCompleted || existing.State == AssistantPlanPartiallyFailed || existing.State == AssistantPlanStale || existing.State == AssistantPlanCancelled {
				response.Receipt = &existing
			}
			writeJSON(w, http.StatusOK, response)
			return
		}
	}
	_, err = s.store.appendUserTurn(conversation.ID, content)
	if err != nil {
		writeAssistantError(w, http.StatusBadRequest, requestID, "invalid_request", err.Error(), false)
		return
	}
	turns, err := s.store.listAssistantTurns(conversation.ID)
	if err != nil {
		writeAssistantError(w, http.StatusInternalServerError, requestID, "storage", "load assistant turns", false)
		return
	}
	assistantContext, err := s.store.assistantContext(s.store.defaultWorkspaceID(), AssistantContextOptions{})
	if err != nil {
		writeAssistantError(w, http.StatusInternalServerError, requestID, "storage", "load assistant context", false)
		return
	}
	plan, providerErr := s.planAssistantTurn(r.Context(), content, turns, assistantContext)
	if providerErr != nil {
		// Retain the failed planning result separately from the source turn. The
		// persisted record contains only a safe class/request reference.
		failure, _ := json.Marshal(map[string]any{"requestId": requestID, "class": providerErr.Class, "retryable": providerErr.Retryable})
		_, _ = s.store.appendAssistantTurn(conversation.ID, "I could not prepare a plan. Please retry.", failure, requestID, "openai-compatible", s.cfg.AIModel)
		log.Printf("assistant planning failed request_id=%s class=%s status=%d retryable=%t", requestID, providerErr.Class, providerErr.Status, providerErr.Retryable)
		status := http.StatusBadGateway
		if providerErr.Class == "rate_limited" {
			status = http.StatusTooManyRequests
		} else if providerErr.Class == "upstream_unavailable" {
			status = http.StatusServiceUnavailable
		} else if providerErr.Class == "timeout" {
			status = http.StatusGatewayTimeout
		}
		message := "assistant provider request failed; retry this request"
		if providerErr.Class == "authentication" {
			message = "assistant provider authentication failed"
		}
		writeAssistantError(w, status, requestID, providerErr.Class, message, providerErr.Retryable)
		return
	}

	planJSON, err := json.Marshal(plan)
	if err != nil {
		writeAssistantError(w, http.StatusBadGateway, requestID, "malformed_output", "assistant provider returned an invalid plan", true)
		return
	}
	turnContent := plan.IntentSummary
	if plan.Kind == "answer" && strings.TrimSpace(plan.Reply) != "" {
		turnContent = plan.Reply
	}
	assistantTurn, err := s.store.appendAssistantTurn(conversation.ID, turnContent, planJSON, requestID, "openai-compatible", s.cfg.AIModel)
	if err != nil {
		writeAssistantError(w, http.StatusInternalServerError, requestID, "storage", "save assistant plan", false)
		return
	}
	if plan.Kind == "clarification" || (plan.Kind == "" && len(plan.Actions) == 0 && len(plan.AmbiguityQuestions) > 0) {
		writeJSON(w, http.StatusOK, assistantPlanResponse{RequestID: requestID, Kind: plan.Kind, Reply: plan.Reply, ClarificationQuestions: plan.AmbiguityQuestions})
		return
	}
	if plan.Kind == "answer" {
		writeJSON(w, http.StatusOK, assistantPlanResponse{RequestID: requestID, Kind: plan.Kind, Reply: plan.Reply})
		return
	}
	key := strings.TrimSpace(request.IdempotencyKey)
	if key == "" {
		key = requestID
	}
	record, err := s.assistantExecutor().Preview(AssistantPlanRequest{WorkspaceID: conversation.WorkspaceID, ConversationID: conversation.ID, TurnID: assistantTurn.ID, IdempotencyKey: key, SourceRequestHash: sourceRequestHash, Actor: "assistant", Plan: plan})
	if err != nil {
		if strings.Contains(err.Error(), "idempotency key already used") {
			writeAssistantError(w, http.StatusConflict, requestID, "idempotency_conflict", "idempotency key already used for a different request", false)
			return
		}
		writeAssistantError(w, http.StatusBadGateway, requestID, "invalid_plan", "assistant provider returned a plan that could not be validated", true)
		return
	}
	response := assistantPlanResponse{RequestID: requestID, Kind: plan.Kind, Reply: plan.Reply, Plan: &record}
	if !record.RequiresConfirmation {
		// This is the only automatic write path. It is deliberately invoked once;
		// provider retries never reach this branch and execution is idempotent.
		receipt, executeErr := s.assistantExecutor().Execute(AssistantExecuteRequest{WorkspaceID: conversation.WorkspaceID, PlanID: record.ID, Actor: "assistant"})
		if executeErr != nil {
			writeAssistantError(w, http.StatusInternalServerError, requestID, "execution", "assistant plan execution failed", false)
			return
		}
		response.Receipt = &receipt
		if receiptJSON, marshalErr := json.Marshal(receipt); marshalErr == nil {
			_, _ = s.store.appendAssistantTurnRecord(conversation.ID, "action", "Agenda 执行回执", receiptJSON, requestID, "openai-compatible", s.cfg.AIModel)
		}
		if receipt.State == AssistantPlanCompleted {
			if memoryErr := s.store.learnDurableMemory(content); memoryErr != nil {
				log.Printf("assistant memory learning failed request_id=%s error=%v", requestID, memoryErr)
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func assistantSourceRequestHash(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}

func (s *Server) assistantExecutor() *AssistantExecutor {
	if s.executor != nil {
		return s.executor
	}
	return NewAssistantExecutor(s.store)
}

func (s *Server) planAssistantTurn(parent context.Context, userText string, turns []AssistantTurn, assistantContext AssistantContext) (AssistantPlan, *assistantProviderError) {
	if s.ai == nil || strings.TrimSpace(s.ai.apiKey) == "" {
		return AssistantPlan{}, &assistantProviderError{Class: "not_configured", Status: http.StatusServiceUnavailable, Retryable: false}
	}
	historyTurns := turns
	if len(historyTurns) > 0 {
		last := historyTurns[len(historyTurns)-1]
		if last.Role == "user" && strings.TrimSpace(last.Content) == strings.TrimSpace(userText) {
			historyTurns = historyTurns[:len(historyTurns)-1]
		}
	}
	planningTurns := make([]map[string]string, 0, len(historyTurns))
	start := 0
	if len(historyTurns) > 30 {
		start = len(historyTurns) - 30
	}
	for _, turn := range historyTurns[start:] {
		content := turn.Content
		if len(content) > 2048 {
			content = content[:2048]
		}
		planningTurns = append(planningTurns, map[string]string{"role": turn.Role, "content": content})
	}
	contextValue := struct {
		UserText string              `json:"userText"`
		Turns    []map[string]string `json:"turns"`
		Agenda   AssistantContext    `json:"agenda"`
	}{userText, planningTurns, assistantContext}
	contextJSON, err := json.Marshal(contextValue)
	if err != nil {
		return AssistantPlan{}, &assistantProviderError{Class: "context", Status: http.StatusInternalServerError, Retryable: false, Cause: err}
	}
	if len(contextJSON) > assistantContextBodyCap {
		contextValue.Turns = contextValue.Turns[len(contextValue.Turns)/2:]
		contextJSON, _ = json.Marshal(contextValue)
		if len(contextJSON) > assistantContextBodyCap {
			contextValue.Agenda.Events = nil
			contextValue.Agenda.Memories = nil
			contextJSON, _ = json.Marshal(contextValue)
		}
		if len(contextJSON) > assistantContextBodyCap {
			if len(contextValue.Agenda.Items) > 20 {
				contextValue.Agenda.Items = contextValue.Agenda.Items[:20]
			}
			contextJSON, _ = json.Marshal(contextValue)
		}
	}
	return s.ai.plan(parent, contextJSON)
}

func (a *AIClient) plan(parent context.Context, contextJSON []byte) (AssistantPlan, *assistantProviderError) {
	prompt := `You are a personal agenda AI that decides what the user wants. Return one concise JSON object with kind (answer|query|plan|clarification), reply, intentSummary, ambiguityQuestions, riskLevel (low|medium|high), requiresConfirmation, and actions. Do not include markdown, explanations, or additional keys. Keep action payloads minimal. Every action must include a unique non-empty actionId string. Payload shapes are exact: create_project uses {project:{name,description?}}, update_project uses {projectId,changes:{...}}, create_item uses {item:{type,title,...}}, update_item uses {itemId,changes:{...}}, and list_items uses {status?,projectId?}. Use kind=answer for ordinary questions or conversation and put the direct answer in reply; use kind=query for read-only agenda questions and list_items actions; use kind=plan for changes; use kind=clarification when required details are missing. Allowed operations are list_items, create_item, update_item, complete_item, reopen_item, archive_item, set_reminder, create_project, update_project, merge_projects, move_items_to_project. Use list_items for read-only questions about current work or items; its payload may include only an optional status (inbox|todo|doing|done|cancelled|archived) and/or projectId. Never invent operations, IDs, or fields. Never use a mutation operation to answer a read-only question. answer and clarification must have no actions; plan/query must have at least one action. Ask clarification through ambiguityQuestions when needed.`
	payload := map[string]any{
		"model": a.model,
		"messages": []map[string]any{
			{"role": "system", "content": prompt},
			{"role": "user", "content": string(contextJSON)},
		},
		"response_format":       map[string]string{"type": "json_object"},
		"temperature":           0.1,
		"max_completion_tokens": 4096,
	}
	var last *assistantProviderError
	for attempt := 0; attempt <= assistantPlanningRetries; attempt++ {
		result, providerErr := a.planRequest(parent, payload)
		if providerErr == nil {
			return result, nil
		}
		last = providerErr
		if !providerErr.Retryable || attempt == assistantPlanningRetries {
			break
		}
	}
	return AssistantPlan{}, last
}

func (a *AIClient) planRequest(parent context.Context, payload map[string]any) (AssistantPlan, *assistantProviderError) {
	// loadConfig validates the operator's private proxy opt-in at startup. Keep
	// this request-level check syntax-only so httptest and injected transports
	// can exercise the client without bypassing startup policy in production.
	if err := validateAIBaseURL(a.baseURL, true); err != nil {
		return AssistantPlan{}, &assistantProviderError{Class: "configuration", Status: http.StatusInternalServerError, Cause: err}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AssistantPlan{}, &assistantProviderError{Class: "request", Status: http.StatusInternalServerError, Cause: err}
	}
	ctx, cancel := context.WithTimeout(parent, assistantProviderTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.baseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return AssistantPlan{}, &assistantProviderError{Class: "configuration", Status: http.StatusInternalServerError, Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := a.client
	if client == nil {
		client = &http.Client{Timeout: assistantProviderTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		class := "network"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			class = "timeout"
		}
		return AssistantPlan{}, &assistantProviderError{Class: class, Status: http.StatusGatewayTimeout, Retryable: true, Cause: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, assistantProviderBodyCap+1))
	if err != nil {
		return AssistantPlan{}, &assistantProviderError{Class: "network", Status: http.StatusBadGateway, Retryable: true, Cause: err}
	}
	if len(data) > assistantProviderBodyCap {
		return AssistantPlan{}, &assistantProviderError{Class: "response_too_large", Status: http.StatusBadGateway, Retryable: false}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		class, retryable := "upstream", false
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			class = "authentication"
		case http.StatusTooManyRequests:
			class, retryable = "rate_limited", true
		default:
			if resp.StatusCode >= 500 {
				class, retryable = "upstream_unavailable", true
			}
		}
		return AssistantPlan{}, &assistantProviderError{Class: class, Status: resp.StatusCode, Retryable: retryable}
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Choices) == 0 || strings.TrimSpace(envelope.Choices[0].Message.Content) == "" {
		return AssistantPlan{}, &assistantProviderError{Class: "malformed_output", Status: http.StatusBadGateway, Retryable: true, Cause: err}
	}
	content := normalizeAssistantModelContent(envelope.Choices[0].Message.Content)
	plan, err := decodeAssistantModelPlan([]byte(content))
	if err != nil {
		return AssistantPlan{}, &assistantProviderError{Class: "malformed_output", Status: http.StatusBadGateway, Retryable: true, Cause: err}
	}
	return plan, nil
}

// normalizeAssistantModelContent tolerates provider formatting around the
// contract JSON while leaving the strict plan/action validation unchanged.
func normalizeAssistantModelContent(content string) string {
	value := strings.TrimSpace(content)
	for {
		start := strings.Index(value, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(value[start+len("<think>"):], "</think>")
		if end < 0 {
			value = strings.TrimSpace(value[:start])
			break
		}
		end += start + len("<think>") + len("</think>")
		value = strings.TrimSpace(value[:start] + value[end:])
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, "```json"))
	value = strings.TrimSpace(strings.TrimPrefix(value, "```"))
	value = strings.TrimSpace(strings.TrimSuffix(value, "```"))
	if object, ok := extractAssistantJSONObject(value); ok {
		return object
	}
	return value
}

func extractAssistantJSONObject(value string) (string, bool) {
	start := strings.IndexByte(value, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(value); i++ {
		ch := value[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(value[start : i+1]), true
			}
		}
	}
	return "", false
}

// decodeAssistantModelPlan accepts the normal closed plan and the deliberately
// action-free clarification form. Clarifications are persisted as assistant
// turns and must be answered before an executable plan is created.
func decodeAssistantModelPlan(data []byte) (AssistantPlan, error) {
	data = normalizeAssistantActionIDs(data)
	plan, err := DecodeAssistantPlan(data)
	if err == nil {
		if plan.Kind == "" {
			plan.Kind = "plan"
			if len(plan.Actions) > 0 {
				readOnly := true
				for _, action := range plan.Actions {
					if action.Operation != AssistantListItems {
						readOnly = false
						break
					}
				}
				if readOnly {
					plan.Kind = "query"
				}
			}
		}
		return plan, nil
	}
	var clarification struct {
		Kind                 string             `json:"kind"`
		Reply                string             `json:"reply"`
		IntentSummary        string             `json:"intentSummary"`
		AmbiguityQuestions   []string           `json:"ambiguityQuestions"`
		RiskLevel            AssistantRiskLevel `json:"riskLevel"`
		RequiresConfirmation bool               `json:"requiresConfirmation"`
		Actions              []json.RawMessage  `json:"actions"`
	}
	if strictErr := decodeStrict(data, &clarification); strictErr != nil {
		return AssistantPlan{}, err
	}
	if clarification.Kind == "answer" {
		if strings.TrimSpace(clarification.IntentSummary) == "" || strings.TrimSpace(clarification.Reply) == "" || len(clarification.Actions) != 0 || len(clarification.AmbiguityQuestions) != 0 {
			return AssistantPlan{}, err
		}
		if clarification.RiskLevel != AssistantRiskLow && clarification.RiskLevel != AssistantRiskMedium && clarification.RiskLevel != AssistantRiskHigh {
			return AssistantPlan{}, err
		}
		return AssistantPlan{Kind: "answer", Reply: clarification.Reply, IntentSummary: clarification.IntentSummary, RiskLevel: clarification.RiskLevel}, nil
	}
	if (clarification.Kind != "" && clarification.Kind != "clarification") || strings.TrimSpace(clarification.IntentSummary) == "" || len(clarification.AmbiguityQuestions) == 0 || len(clarification.Actions) != 0 {
		return AssistantPlan{}, err
	}
	switch clarification.RiskLevel {
	case AssistantRiskLow, AssistantRiskMedium, AssistantRiskHigh:
	default:
		return AssistantPlan{}, err
	}
	for i, question := range clarification.AmbiguityQuestions {
		if strings.TrimSpace(question) == "" {
			return AssistantPlan{}, fmt.Errorf("ambiguityQuestions[%d] must not be empty", i)
		}
	}
	return AssistantPlan{Kind: "clarification", Reply: clarification.Reply, IntentSummary: clarification.IntentSummary, AmbiguityQuestions: clarification.AmbiguityQuestions, RiskLevel: clarification.RiskLevel, RequiresConfirmation: clarification.RequiresConfirmation}, nil
}

// Some compatible models omit the internal action identifier when emitting a
// larger batch. It carries no user semantics, so fill only missing IDs while
// preserving every other strict action validation rule.
func normalizeAssistantActionIDs(data []byte) []byte {
	var envelope struct {
		Actions []map[string]json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Actions) == 0 {
		return data
	}
	changed := false
	for i, action := range envelope.Actions {
		if _, hasOperation := action["operation"]; !hasOperation {
			if raw, hasAction := action["action"]; hasAction {
				action["operation"] = raw
				delete(action, "action")
				changed = true
			}
		}
		var id string
		if raw, ok := action["actionId"]; ok {
			_ = json.Unmarshal(raw, &id)
		}
		if strings.TrimSpace(id) == "" {
			envelope.Actions[i]["actionId"], _ = json.Marshal(fmt.Sprintf("action-%d", i+1))
			changed = true
		}
		if normalizeAssistantProjectPayload(action) {
			changed = true
		}
	}
	if !changed {
		return data
	}
	value := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	actions, err := json.Marshal(envelope.Actions)
	if err != nil {
		return data
	}
	value["actions"] = actions
	result, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return result
}

func normalizeAssistantProjectPayload(action map[string]json.RawMessage) bool {
	var operation string
	if raw, ok := action["operation"]; !ok || json.Unmarshal(raw, &operation) != nil || operation != string(AssistantCreateProject) {
		return false
	}
	var payload map[string]json.RawMessage
	if raw, ok := action["payload"]; !ok || json.Unmarshal(raw, &payload) != nil {
		return false
	}
	if _, wrapped := payload["project"]; wrapped {
		return false
	}
	project := map[string]json.RawMessage{}
	for _, key := range []string{"name", "description", "status", "priority", "color", "goal", "deadline", "reviewAt"} {
		if raw, ok := payload[key]; ok {
			project[key] = raw
		}
	}
	if raw, ok := payload["title"]; ok {
		if _, exists := project["name"]; !exists {
			project["name"] = raw
		}
	}
	if len(project) == 0 {
		return false
	}
	wrapper, err := json.Marshal(map[string]any{"project": project})
	if err != nil {
		return false
	}
	action["payload"] = wrapper
	return true
}

func (s *Server) handleAssistantConfirmPlan(w http.ResponseWriter, r *http.Request) {
	requestID := assistantRequestID(r)
	planID := r.PathValue("id")
	record, err := s.assistantExecutor().Execute(AssistantExecuteRequest{WorkspaceID: s.store.defaultWorkspaceID(), PlanID: planID, Confirmed: true, Actor: "assistant"})
	if err != nil {
		assistantWritePlanError(w, requestID, err)
		return
	}
	if record.State == AssistantPlanCompleted && record.ConversationID != "" {
		if turns, turnErr := s.store.listAssistantTurns(record.ConversationID); turnErr == nil {
			for _, turn := range turns {
				if turn.ID == record.TurnID && turn.Role == "user" {
					if memoryErr := s.store.learnDurableMemory(turn.Content); memoryErr != nil {
						log.Printf("assistant memory learning failed request_id=%s error=%v", requestID, memoryErr)
					}
					break
				}
			}
		}
	}
	if receiptJSON, marshalErr := json.Marshal(record); marshalErr == nil {
		_, _ = s.store.appendAssistantTurnRecord(record.ConversationID, "action", "Agenda 执行回执", receiptJSON, requestID, "openai-compatible", s.cfg.AIModel)
	}
	writeJSON(w, http.StatusOK, assistantPlanResponse{RequestID: requestID, Plan: &record, Receipt: &record})
}

func (s *Server) handleAssistantCancelPlan(w http.ResponseWriter, r *http.Request) {
	requestID := assistantRequestID(r)
	record, err := s.assistantExecutor().Cancel(s.store.defaultWorkspaceID(), r.PathValue("id"))
	if err != nil {
		assistantWritePlanError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, assistantPlanResponse{RequestID: requestID, Plan: &record, Receipt: &record})
}

func (s *Server) handleAssistantGetPlan(w http.ResponseWriter, r *http.Request) {
	record, err := s.assistantExecutor().store.getAssistantPlan(s.store.defaultWorkspaceID(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "assistant plan not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get assistant plan")
		return
	}
	writeJSON(w, http.StatusOK, assistantPlanResponse{RequestID: assistantRequestID(r), Plan: &record, Receipt: &record})
}

func (s *Server) handleAssistantUndoPlan(w http.ResponseWriter, r *http.Request) {
	requestID := assistantRequestID(r)
	record, err := s.assistantExecutor().Undo(s.store.defaultWorkspaceID(), r.PathValue("id"))
	if err != nil {
		assistantWritePlanError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, assistantPlanResponse{RequestID: requestID, Plan: &record, Receipt: &record})
}

func assistantWritePlanError(w http.ResponseWriter, requestID string, err error) {
	status, class, retryable := http.StatusInternalServerError, "execution", false
	if errors.Is(err, sql.ErrNoRows) {
		status, class = http.StatusNotFound, "not_found"
	} else if errors.Is(err, errConflict) || strings.Contains(strings.ToLower(err.Error()), "confirmation") {
		status, class = http.StatusConflict, "conflict"
	}
	writeAssistantError(w, status, requestID, class, "assistant plan operation failed", retryable)
}
