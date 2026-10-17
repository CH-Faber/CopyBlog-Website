package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeAssistantModelContent(t *testing.T) {
	content := "<think>先分析用户的多个项目</think>\n下面是结果：\n```json\n{\"kind\":\"answer\",\"reply\":\"已整理\",\"intentSummary\":\"整理项目\",\"riskLevel\":\"low\",\"actions\":[]}\n```\n"
	normalized := normalizeAssistantModelContent(content)
	plan, err := decodeAssistantModelPlan([]byte(normalized))
	if err != nil {
		t.Fatalf("normalized provider content should decode: %v; content=%q", err, normalized)
	}
	if plan.Kind != "answer" || plan.Reply != "已整理" {
		t.Fatalf("unexpected normalized plan: %#v", plan)
	}
}

func TestNormalizeAssistantActionIDs(t *testing.T) {
	plan, err := decodeAssistantModelPlan([]byte(`{"kind":"query","intentSummary":"查询项目","riskLevel":"low","requiresConfirmation":false,"actions":[{"action":"list_items","payload":{}}]}`))
	if err != nil || plan.Actions[0].Operation != AssistantListItems || plan.Actions[0].ActionID != "action-1" {
		t.Fatalf("action alias was not normalized: %v %#v", err, plan)
	}
	plan, err = decodeAssistantModelPlan([]byte(`{"kind":"plan","intentSummary":"整理项目","riskLevel":"medium","requiresConfirmation":true,"actions":[{"operation":"create_project","payload":{"title":"项目一","description":"描述一"}},{"actionId":"existing","operation":"create_project","payload":{"project":{"name":"项目二"}}}]}`))
	if err != nil {
		t.Fatalf("missing action id should be normalized: %v", err)
	}
	if plan.Actions[0].ActionID != "action-1" || plan.Actions[1].ActionID != "existing" {
		t.Fatalf("unexpected action ids: %#v", plan.Actions)
	}
	project := plan.Actions[0].Payload.(*AssistantCreateProjectPayload)
	if project.Project.Name == nil || *project.Project.Name != "项目一" || project.Project.Description == nil || *project.Project.Description != "描述一" {
		t.Fatalf("project payload was not normalized: %#v", project)
	}
}

func TestAssistantHTTPConversationPlanningAndBothPrefixes(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Fatalf("missing upstream authorization")
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"intentSummary\":\"create task\",\"riskLevel\":\"low\",\"actions\":[{\"actionId\":\"create\",\"operation\":\"create_item\",\"payload\":{\"item\":{\"type\":\"task\",\"title\":\"planned task\",\"timezone\":\"Asia/Shanghai\"}}}]}"}}]}`))
	}))
	defer upstream.Close()

	for _, prefix := range []string{"/api/organizer/v1", "/api/agenda/v1"} {
		t.Run(prefix, func(t *testing.T) {
			server, handler, password := testServer(t)
			server.cfg.AIBaseURL = upstream.URL + "/v1"
			server.cfg.AIAPIKey = "upstream-secret"
			server.cfg.AIModel = "test-model"
			server.ai.baseURL = server.cfg.AIBaseURL
			server.ai.apiKey = server.cfg.AIAPIKey
			server.ai.model = server.cfg.AIModel
			server.ai.allowPrivateProxy = true
			login := requestJSON(t, handler, http.MethodPost, prefix+"/auth/device", map[string]string{"password": password}, nil)
			if login.Code != http.StatusCreated {
				t.Fatalf("device login failed: %d %s", login.Code, login.Body.String())
			}
			var auth struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(login.Body.Bytes(), &auth); err != nil || auth.Token == "" {
				t.Fatalf("invalid auth response: %v", err)
			}
			created := bearerJSON(t, handler, http.MethodPost, prefix+"/assistant/conversations", nil, auth.Token)
			if created.Code != http.StatusCreated {
				t.Fatalf("create conversation failed: %d %s", created.Code, created.Body.String())
			}
			var conversation AssistantConversation
			if err := json.Unmarshal(created.Body.Bytes(), &conversation); err != nil || conversation.ID == "" {
				t.Fatalf("invalid conversation: %v %s", err, created.Body.String())
			}
			turn := bearerJSON(t, handler, http.MethodPost, prefix+"/assistant/conversations/"+conversation.ID+"/turns", map[string]string{"content": "add a planned task", "idempotencyKey": "http-plan-" + strings.Trim(prefix, "/")}, auth.Token)
			if turn.Code != http.StatusOK {
				t.Fatalf("plan turn failed: %d %s", turn.Code, turn.Body.String())
			}
			var response struct {
				Plan    AssistantPlanRecord  `json:"plan"`
				Receipt *AssistantPlanRecord `json:"receipt"`
			}
			if err := json.Unmarshal(turn.Body.Bytes(), &response); err != nil || response.Plan.ID == "" || response.Plan.State != AssistantPlanAwaitingConfirmation || !response.Plan.RequiresConfirmation || response.Receipt != nil {
				t.Fatalf("unexpected plan response: %v %#v", err, response)
			}
			confirmed := bearerJSON(t, handler, http.MethodPost, prefix+"/assistant/plans/"+response.Plan.ID+"/confirm", nil, auth.Token)
			if confirmed.Code != http.StatusOK {
				t.Fatalf("plan confirmation failed: %d %s", confirmed.Code, confirmed.Body.String())
			}
			var confirmation struct {
				Plan    AssistantPlanRecord  `json:"plan"`
				Receipt *AssistantPlanRecord `json:"receipt"`
			}
			if err := json.Unmarshal(confirmed.Body.Bytes(), &confirmation); err != nil || confirmation.Plan.State != AssistantPlanCompleted || confirmation.Receipt == nil || confirmation.Receipt.State != AssistantPlanCompleted {
				t.Fatalf("unexpected confirmation response: %v %#v", err, confirmation)
			}
			conversationRead := bearerJSON(t, handler, http.MethodGet, prefix+"/assistant/conversations/"+conversation.ID, nil, auth.Token)
			if conversationRead.Code != http.StatusOK || !strings.Contains(conversationRead.Body.String(), "add a planned task") {
				t.Fatalf("conversation read failed: %d %s", conversationRead.Code, conversationRead.Body.String())
			}
		})
	}
	if calls.Load() != 2 {
		t.Fatalf("provider calls=%d, want two prefixes", calls.Load())
	}
}

func TestAssistantHTTPProviderFailureDoesNotLeakSecret(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Bearer upstream-secret should not be returned"))
	}))
	defer upstream.Close()
	server, handler, password := testServer(t)
	server.ai.baseURL = upstream.URL
	server.ai.apiKey = "upstream-secret"
	server.ai.allowPrivateProxy = true
	login := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/auth/device", map[string]string{"password": password}, nil)
	var auth struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &auth)
	created := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations", nil, auth.Token)
	var conversation AssistantConversation
	_ = json.Unmarshal(created.Body.Bytes(), &conversation)
	failed := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations/"+conversation.ID+"/turns", map[string]string{"content": "try"}, auth.Token)
	if failed.Code != http.StatusBadGateway || strings.Contains(failed.Body.String(), "upstream-secret") || failed.Header().Get("X-Request-ID") == "" {
		t.Fatalf("unsafe provider failure response: %d %s", failed.Code, failed.Body.String())
	}
}

func TestAssistantHTTPAnswerReturnsReplyWithoutCreatingPlan(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"kind\":\"answer\",\"reply\":\"把任务拆小并设定开始时间，通常会更容易启动。\",\"intentSummary\":\"回答如何开始任务\",\"ambiguityQuestions\":[],\"riskLevel\":\"low\",\"requiresConfirmation\":false,\"actions\":[]}"}}]}`))
	}))
	defer upstream.Close()
	server, handler, password := testServer(t)
	server.ai.baseURL, server.ai.apiKey, server.ai.allowPrivateProxy = upstream.URL, "key", true
	login := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/auth/device", map[string]string{"password": password}, nil)
	var auth struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &auth); err != nil || auth.Token == "" {
		t.Fatalf("invalid auth response: %v", err)
	}
	conversationResponse := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations", nil, auth.Token)
	var conversation AssistantConversation
	if err := json.Unmarshal(conversationResponse.Body.Bytes(), &conversation); err != nil {
		t.Fatal(err)
	}
	response := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations/"+conversation.ID+"/turns", map[string]string{"content": "我为什么难以开始任务？", "idempotencyKey": "direct-answer"}, auth.Token)
	var result struct {
		Kind  string `json:"kind"`
		Reply string `json:"reply"`
		Plan  any    `json:"plan"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Kind != "answer" || result.Reply == "" || result.Plan != nil {
		t.Fatalf("unexpected direct answer response: %d %s", response.Code, response.Body.String())
	}
	var plans int
	if err := server.store.db.QueryRow("SELECT COUNT(*) FROM assistant_plans").Scan(&plans); err != nil || plans != 0 {
		t.Fatalf("direct answer created plans=%d err=%v", plans, err)
	}
}

func TestAssistantHTTPCaptureTurnsAreDurableAndIdempotent(t *testing.T) {
	server, handler, password := testServer(t)
	login := requestJSON(t, handler, http.MethodPost, "/api/agenda/v1/auth/device", map[string]string{"password": password}, nil)
	var auth struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &auth); err != nil || auth.Token == "" {
		t.Fatalf("invalid auth response: %v", err)
	}
	created := bearerJSON(t, handler, http.MethodPost, "/api/agenda/v1/assistant/conversations", nil, auth.Token)
	var conversation AssistantConversation
	if err := json.Unmarshal(created.Body.Bytes(), &conversation); err != nil {
		t.Fatal(err)
	}
	capture, err := server.store.createCapture(Capture{SourceType: "image", RawText: "下周完成实验报告", AttachmentName: "schedule.png", AttachmentMime: "image/png", AttachmentPath: "attachments/schedule.png"})
	if err != nil {
		t.Fatal(err)
	}
	result := `{"items":[{"type":"task","title":"完成实验报告","timezone":"Asia/Shanghai","allDay":false,"priority":1,"confidence":0.9}]}`
	if _, err := server.store.db.Exec("UPDATE captures SET status='needs_review', ai_result=? WHERE id=?", result, capture.ID); err != nil {
		t.Fatal(err)
	}
	path := "/api/agenda/v1/assistant/conversations/" + conversation.ID + "/captures"
	for i := 0; i < 2; i++ {
		response := bearerJSON(t, handler, http.MethodPost, path, map[string]string{"captureId": capture.ID}, auth.Token)
		if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "完成实验报告") {
			t.Fatalf("capture turn failed: %d %s", response.Code, response.Body.String())
		}
	}
	turns, err := server.store.listAssistantTurns(conversation.ID)
	if err != nil || len(turns) != 2 || turns[0].Role != "user" || turns[1].Role != "assistant" || turns[0].StructuredResultRef != capture.ID || turns[1].StructuredResultRef != capture.ID {
		t.Fatalf("capture turns were not stored exactly once: %#v %v", turns, err)
	}
	loaded, err := server.store.getAssistantConversation(conversation.ID)
	if err != nil || !strings.Contains(loaded.Title, "schedule.png") {
		t.Fatalf("capture did not title conversation: %#v %v", loaded, err)
	}
}

func TestAssistantHTTPIdempotencyIsScopedAndSourceBound(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"intentSummary\":\"create task\",\"riskLevel\":\"low\",\"actions\":[{\"actionId\":\"create\",\"operation\":\"create_item\",\"payload\":{\"item\":{\"type\":\"task\",\"title\":\"one\",\"timezone\":\"Asia/Shanghai\"}}}]}"}}]}`))
	}))
	defer upstream.Close()
	server, handler, password := testServer(t)
	server.ai.baseURL, server.ai.apiKey, server.ai.allowPrivateProxy = upstream.URL, "key", true
	login := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/auth/device", map[string]string{"password": password}, nil)
	var auth struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &auth); err != nil || auth.Token == "" {
		t.Fatalf("invalid auth response: %v", err)
	}
	createConversation := func() AssistantConversation {
		record := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations", nil, auth.Token)
		if record.Code != http.StatusCreated {
			t.Fatalf("conversation create failed: %d %s", record.Code, record.Body.String())
		}
		var conversation AssistantConversation
		if err := json.Unmarshal(record.Body.Bytes(), &conversation); err != nil {
			t.Fatal(err)
		}
		return conversation
	}
	first := createConversation()
	second := createConversation()
	firstTurn := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations/"+first.ID+"/turns", map[string]string{"content": "first request", "idempotencyKey": "same-key"}, auth.Token)
	if firstTurn.Code != http.StatusOK {
		t.Fatalf("first turn failed: %d %s", firstTurn.Code, firstTurn.Body.String())
	}
	differentText := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations/"+first.ID+"/turns", map[string]string{"content": "different request", "idempotencyKey": "same-key"}, auth.Token)
	if differentText.Code != http.StatusConflict || !strings.Contains(differentText.Body.String(), "idempotency_conflict") {
		t.Fatalf("different source text reused key: %d %s", differentText.Code, differentText.Body.String())
	}
	differentConversation := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations/"+second.ID+"/turns", map[string]string{"content": "other conversation", "idempotencyKey": "same-key"}, auth.Token)
	if differentConversation.Code != http.StatusConflict || !strings.Contains(differentConversation.Body.String(), "idempotency_conflict") {
		t.Fatalf("different conversation reused key: %d %s", differentConversation.Code, differentConversation.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("idempotency conflicts reached provider: calls=%d", calls.Load())
	}
}

func TestAssistantHTTPClarificationConfirmationAndCancellation(t *testing.T) {
	var mode atomic.Int32
	var targetID atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mode.Load() == 0 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"intentSummary\":\"need a date\",\"ambiguityQuestions\":[\"Which date?\"],\"riskLevel\":\"low\",\"actions\":[]}"}}]}`))
			return
		}
		id, _ := targetID.Load().(string)
		body := `{"choices":[{"message":{"content":"{\"intentSummary\":\"archive item\",\"riskLevel\":\"high\",\"requiresConfirmation\":true,\"actions\":[{\"actionId\":\"archive\",\"operation\":\"archive_item\",\"payload\":{\"itemId\":\"` + id + `\"}}]}"}}]}`
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()
	server, handler, password := testServer(t)
	server.ai.baseURL, server.ai.apiKey, server.ai.allowPrivateProxy = upstream.URL, "key", true
	login := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/auth/device", map[string]string{"password": password}, nil)
	var auth struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &auth)
	conversation := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations", nil, auth.Token)
	var c AssistantConversation
	_ = json.Unmarshal(conversation.Body.Bytes(), &c)
	clarification := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations/"+c.ID+"/turns", map[string]string{"content": "archive something"}, auth.Token)
	if clarification.Code != http.StatusOK || !strings.Contains(clarification.Body.String(), "Which date?") {
		t.Fatalf("clarification failed: %d %s", clarification.Code, clarification.Body.String())
	}
	item := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/items", Candidate{Type: "task", Title: "archive me", Timezone: "Asia/Shanghai"}, auth.Token)
	var agendaItem Item
	_ = json.Unmarshal(item.Body.Bytes(), &agendaItem)
	targetID.Store(agendaItem.ID)
	mode.Store(1)
	planned := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations/"+c.ID+"/turns", map[string]string{"content": "archive it", "idempotencyKey": "archive-http"}, auth.Token)
	var response struct {
		Plan AssistantPlanRecord `json:"plan"`
	}
	_ = json.Unmarshal(planned.Body.Bytes(), &response)
	if planned.Code != http.StatusOK || response.Plan.State != AssistantPlanAwaitingConfirmation {
		t.Fatalf("confirmation plan failed: %d %#v %s", planned.Code, response.Plan, planned.Body.String())
	}
	confirmed := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/plans/"+response.Plan.ID+"/confirm", nil, auth.Token)
	if confirmed.Code != http.StatusOK || !strings.Contains(confirmed.Body.String(), "completed") {
		t.Fatalf("confirmation failed: %d %s", confirmed.Code, confirmed.Body.String())
	}
	itemRead := bearerJSON(t, handler, http.MethodGet, "/api/organizer/v1/items/"+agendaItem.ID, nil, auth.Token)
	if !strings.Contains(itemRead.Body.String(), `"status":"archived"`) {
		t.Fatalf("confirmed archive did not apply: %s", itemRead.Body.String())
	}

	item2 := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/items", Candidate{Type: "task", Title: "keep me", Timezone: "Asia/Shanghai"}, auth.Token)
	var agendaItem2 Item
	_ = json.Unmarshal(item2.Body.Bytes(), &agendaItem2)
	targetID.Store(agendaItem2.ID)
	planned = bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/conversations/"+c.ID+"/turns", map[string]string{"content": "archive another", "idempotencyKey": "archive-http-cancel"}, auth.Token)
	_ = json.Unmarshal(planned.Body.Bytes(), &response)
	cancelled := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/assistant/plans/"+response.Plan.ID+"/cancel", nil, auth.Token)
	if cancelled.Code != http.StatusOK || !strings.Contains(cancelled.Body.String(), "cancelled") {
		t.Fatalf("cancellation failed: %d %s", cancelled.Code, cancelled.Body.String())
	}
	itemRead = bearerJSON(t, handler, http.MethodGet, "/api/organizer/v1/items/"+agendaItem2.ID, nil, auth.Token)
	if !strings.Contains(itemRead.Body.String(), `"status":"todo"`) {
		t.Fatalf("cancelled archive mutated item: %s", itemRead.Body.String())
	}
}

func TestAssistantProviderErrorClassificationAndRetry(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		wantClass string
		wantRetry bool
		wantHTTP  int
	}{
		{"auth", http.StatusUnauthorized, "authentication", false, http.StatusBadGateway},
		{"rate limit", http.StatusTooManyRequests, "rate_limited", true, http.StatusTooManyRequests},
		{"server", http.StatusBadGateway, "upstream_unavailable", true, http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.status)
			}))
			defer upstream.Close()
			client := &AIClient{baseURL: upstream.URL, apiKey: "key", model: "model", client: upstream.Client(), allowPrivateProxy: true}
			_, err := client.plan(context.Background(), []byte(`{"userText":"x"}`))
			if err == nil || err.Class != test.wantClass || err.Retryable != test.wantRetry {
				t.Fatalf("classification: %#v", err)
			}
			if calls.Load() != int32(1+boolToInt(test.wantRetry)) {
				t.Fatalf("calls=%d", calls.Load())
			}
			status := http.StatusBadGateway
			if test.wantClass == "rate_limited" {
				status = http.StatusTooManyRequests
			} else if test.wantClass == "upstream_unavailable" {
				status = http.StatusServiceUnavailable
			}
			if status != test.wantHTTP {
				t.Fatalf("test mapping setup mismatch: %d", status)
			}
		})
	}
	malformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"}}]}`))
	}))
	defer malformed.Close()
	client := &AIClient{baseURL: malformed.URL, apiKey: "key", model: "model", client: malformed.Client(), allowPrivateProxy: true}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.plan(ctx, []byte(`{"userText":"x"}`)); err == nil || err.Class != "malformed_output" || !err.Retryable {
		t.Fatalf("malformed response classification: %#v", err)
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
