package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDecodeAssistantPlanValidActions(t *testing.T) {
	operations := []string{
		`{"actionId":"a1","operation":"create_item","payload":{"item":{"type":"task","title":"Write brief","priority":2,"timezone":"Asia/Shanghai","dueAt":"2026-10-01T10:00:00+08:00"}}}`,
		`{"actionId":"a2","operation":"update_item","expectedItemVersion":3,"payload":{"itemId":"item-1","changes":{"title":"Updated"}}}`,
		`{"actionId":"a3","operation":"complete_item","payload":{"itemId":"item-1"}}`,
		`{"actionId":"a4","operation":"reopen_item","payload":{"itemId":"item-1"}}`,
		`{"actionId":"a5","operation":"archive_item","payload":{"itemId":"item-1"}}`,
		`{"actionId":"a6","operation":"set_reminder","payload":{"itemId":"item-1","reminderAt":"2026-10-01T09:00:00Z","timezone":"UTC"}}`,
		`{"actionId":"a7","operation":"create_project","payload":{"project":{"name":"Launch"}}}`,
		`{"actionId":"a8","operation":"update_project","expectedProjectVersion":2,"payload":{"projectId":"project-1","changes":{"status":"waiting"}}}`,
		`{"actionId":"a9","operation":"merge_projects","expectedProjectVersion":4,"payload":{"sourceProjectId":"project-1","targetProjectId":"project-2","sourceExpectedVersion":5}}`,
		`{"actionId":"a10","operation":"move_items_to_project","expectedProjectVersion":4,"payload":{"itemIds":["item-1","item-2"],"targetProjectId":"project-2","expectedVersions":{"item-1":1,"item-2":3}}}`,
		`{"actionId":"a11","operation":"list_items","payload":{"status":"todo","projectId":"project-1"}}`,
	}
	for i, action := range operations {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			data := `{"intentSummary":"Organize these changes","ambiguityQuestions":[],"riskLevel":"medium","requiresConfirmation":true,"actions":[` + action + `]}`
			plan, err := DecodeAssistantPlan([]byte(data))
			if err != nil {
				t.Fatalf("DecodeAssistantPlan() error = %v", err)
			}
			if len(plan.Actions) != 1 {
				t.Fatalf("got %d actions", len(plan.Actions))
			}
		})
	}
}

func TestDecodeAssistantPlanRejectsInvalidPayloads(t *testing.T) {
	tests := []struct{ name, action, want string }{
		{"unknown operation", `{"actionId":"a","operation":"delete_everything","payload":{}}`, "unknown assistant operation"},
		{"extra action field", `{"actionId":"a","operation":"complete_item","url":"https://example.test","payload":{"itemId":"i"}}`, "unknown field"},
		{"extra payload field", `{"actionId":"a","operation":"complete_item","payload":{"itemId":"i","sql":"delete"}}`, "unknown field"},
		{"duplicate action key", `{"actionId":"a","actionId":"b","operation":"complete_item","payload":{"itemId":"i"}}`, "duplicate JSON field"},
		{"case variant action key", `{"ActionId":"a","operation":"complete_item","payload":{"itemId":"i"}}`, "canonical spelling"},
		{"duplicate payload key", `{"actionId":"a","operation":"complete_item","payload":{"itemId":"i","itemId":"j"}}`, "duplicate JSON field"},
		{"case variant payload key", `{"actionId":"a","operation":"complete_item","payload":{"ItemId":"i"}}`, "canonical spelling"},
		{"missing target", `{"actionId":"a","operation":"complete_item","payload":{}}`, "itemId is required"},
		{"invalid timezone", `{"actionId":"a","operation":"set_reminder","payload":{"itemId":"i","reminderAt":"2026-10-01T09:00:00Z","timezone":"Not/AZone"}}`, "invalid timezone"},
		{"invalid timestamp", `{"actionId":"a","operation":"set_reminder","payload":{"itemId":"i","reminderAt":"tomorrow","timezone":"UTC"}}`, "RFC3339"},
		{"priority too high", `{"actionId":"a","operation":"create_item","payload":{"item":{"type":"task","title":"Task","priority":4}}}`, "priority must be between 0 and 3"},
		{"priority negative", `{"actionId":"a","operation":"create_project","payload":{"project":{"name":"Launch","priority":-1}}}`, "priority must be between 0 and 3"},
		{"duration exceeds store maximum", `{"actionId":"a","operation":"create_item","payload":{"item":{"type":"task","title":"Task","durationMinutes":43201}}}`, "durationMinutes must be between 0 and 43200"},
		{"unknown model capability", `{"actionId":"a","operation":"create_item","payload":{"item":{"type":"task","title":"Task","url":"https://example.test"}}}`, "unknown field"},
		{"invalid list status", `{"actionId":"a","operation":"list_items","payload":{"status":"pending"}}`, "invalid item status"},
		{"wrong payload type", `{"actionId":"a","operation":"complete_item","payload":{"itemId":17}}`, "cannot unmarshal number"},
		{"wrong operation payload", `{"actionId":"a","operation":"complete_item","payload":{"projectId":"p"}}`, "unknown field"},
		{"trailing JSON", `{"actionId":"a","operation":"complete_item","payload":{"itemId":"i"}} {}`, "invalid character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := `{"intentSummary":"Do it","ambiguityQuestions":[],"riskLevel":"low","requiresConfirmation":false,"actions":[` + tt.action + `]}`
			_, err := DecodeAssistantPlan([]byte(data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestDecodeAssistantModelPlanSupportsAnswersAndClassifiesQueries(t *testing.T) {
	answer, err := decodeAssistantModelPlan([]byte(`{"kind":"answer","reply":"拖延可能和任务太大有关。","intentSummary":"回答关于拖延的问题","ambiguityQuestions":[],"riskLevel":"low","requiresConfirmation":false,"actions":[]}`))
	if err != nil || answer.Kind != "answer" || answer.Reply == "" || len(answer.Actions) != 0 {
		t.Fatalf("answer decode: %#v %v", answer, err)
	}
	clarification, err := decodeAssistantModelPlan([]byte(`{"kind":"clarification","reply":"我需要确认一下。","intentSummary":"目标不明确","ambiguityQuestions":["你指哪个项目？"],"riskLevel":"low","requiresConfirmation":false,"actions":[]}`))
	if err != nil || clarification.Kind != "clarification" || len(clarification.AmbiguityQuestions) != 1 {
		t.Fatalf("clarification decode: %#v %v", clarification, err)
	}
	query, err := decodeAssistantModelPlan([]byte(`{"intentSummary":"查询未完成事项","ambiguityQuestions":[],"riskLevel":"low","requiresConfirmation":false,"actions":[{"actionId":"list","operation":"list_items","payload":{"status":"todo"}}]}`))
	if err != nil || query.Kind != "query" {
		t.Fatalf("query classification: %#v %v", query, err)
	}
	if _, err := decodeAssistantModelPlan([]byte(`{"kind":"answer","reply":"Hello","intentSummary":"Hello","ambiguityQuestions":[],"riskLevel":"low","requiresConfirmation":false,"actions":[{"actionId":"list","operation":"list_items","payload":{}}]}`)); err == nil {
		t.Fatal("answer with actions was accepted")
	}
}

func TestDecodeAssistantPlanRejectsOversizedAndInvalidPlans(t *testing.T) {
	action := `{"actionId":"a","operation":"complete_item","payload":{"itemId":"i"}}`
	tooMany := make([]string, maxAssistantPlanActions+1)
	for i := range tooMany {
		tooMany[i] = strings.Replace(action, `"a"`, fmt.Sprintf(`"a%d"`, i), 1)
	}
	data := `{"intentSummary":"Do it","ambiguityQuestions":[],"riskLevel":"low","requiresConfirmation":false,"actions":[` + strings.Join(tooMany, ",") + `]}`
	if _, err := DecodeAssistantPlan([]byte(data)); err == nil || !strings.Contains(err.Error(), "1 to 50 actions") {
		t.Fatalf("oversized plan error = %v", err)
	}
	data = `{"intentSummary":"Do it","ambiguityQuestions":[],"riskLevel":"low","requiresConfirmation":false,"actions":[{"actionId":"a","operation":"move_items_to_project","payload":{"itemIds":["i"],"targetProjectId":"p"}}]}`
	if _, err := DecodeAssistantPlan([]byte(data)); err != nil {
		t.Fatalf("one item batch rejected: %v", err)
	}
	ids := make([]string, maxAssistantMoveItems+1)
	for i := range ids {
		ids[i] = fmt.Sprintf(`"i%d"`, i)
	}
	data = `{"intentSummary":"Do it","ambiguityQuestions":[],"riskLevel":"low","requiresConfirmation":false,"actions":[{"actionId":"a","operation":"move_items_to_project","payload":{"itemIds":[` + strings.Join(ids, ",") + `],"targetProjectId":"p"}}]}`
	if _, err := DecodeAssistantPlan([]byte(data)); err == nil || !strings.Contains(err.Error(), "1 to 100 IDs") {
		t.Fatalf("oversized move error = %v", err)
	}
}

type fakeAssistantTargets struct {
	itemVersions    map[string]int
	projectVersions map[string]int
	workspace       string
}

func (f fakeAssistantTargets) AssistantItemVersion(workspaceID, id string) (int, error) {
	if workspaceID != f.workspace {
		return 0, errors.New("wrong workspace")
	}
	version, ok := f.itemVersions[id]
	if !ok {
		return 0, errors.New("not found")
	}
	return version, nil
}
func (f fakeAssistantTargets) AssistantProjectVersion(workspaceID, id string) (int, error) {
	if workspaceID != f.workspace {
		return 0, errors.New("wrong workspace")
	}
	version, ok := f.projectVersions[id]
	if !ok {
		return 0, errors.New("not found")
	}
	return version, nil
}

func TestAssistantActionValidateTargets(t *testing.T) {
	reader := fakeAssistantTargets{workspace: "w1", itemVersions: map[string]int{"item-1": 3}, projectVersions: map[string]int{"project-1": 2}}
	version := 3
	title := "Updated"
	valid := AssistantAction{ActionID: "a1", Operation: AssistantUpdateItem, ExpectedItemVersion: &version, Payload: &AssistantUpdateItemPayload{ItemID: "item-1", Changes: AssistantItemFields{Title: &title}}}
	if err := valid.ValidateTargets("w1", reader); err != nil {
		t.Fatalf("valid target: %v", err)
	}
	if err := valid.ValidateTargets("w2", reader); err == nil || !strings.Contains(err.Error(), "missing from workspace") {
		t.Fatalf("cross-workspace target error = %v", err)
	}
	stale := 4
	valid.ExpectedItemVersion = &stale
	if err := valid.ValidateTargets("w1", reader); err == nil || !strings.Contains(err.Error(), "stale version") {
		t.Fatalf("stale item error = %v", err)
	}
	missing := AssistantAction{ActionID: "a2", Operation: AssistantCompleteItem, Payload: &AssistantItemTargetPayload{ItemID: "missing"}}
	if err := missing.ValidateTargets("w1", reader); err == nil || !strings.Contains(err.Error(), "missing from workspace") {
		t.Fatalf("missing target error = %v", err)
	}
	projectVersion := 1
	projectName := "Launch"
	projectAction := AssistantAction{ActionID: "a3", Operation: AssistantUpdateProject, ExpectedProjectVersion: &projectVersion, Payload: &AssistantUpdateProjectPayload{ProjectID: "project-1", Changes: AssistantProjectFields{Name: &projectName}}}
	if err := projectAction.ValidateTargets("w1", reader); err == nil || !strings.Contains(err.Error(), "stale version") {
		t.Fatalf("stale project error = %v", err)
	}
}
