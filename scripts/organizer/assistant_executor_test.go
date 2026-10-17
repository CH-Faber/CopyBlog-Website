package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func assistantPlanFor(actions ...AssistantAction) AssistantPlan {
	return AssistantPlan{IntentSummary: "organize request", RiskLevel: AssistantRiskLow, Actions: actions}
}

func assistantCreateAction(title string) AssistantAction {
	typ, zone := "task", "UTC"
	return AssistantAction{ActionID: "a1", Operation: AssistantCreateItem, Payload: &AssistantCreateItemPayload{Item: AssistantItemFields{Type: &typ, Title: &title, Timezone: &zone}}}
}

func TestAssistantExecutorListItemsIsReadOnly(t *testing.T) {
	store := testStore(t)
	created, err := store.createItems("", []Candidate{
		{Type: "task", Title: "Open task", Timezone: "UTC"},
		{Type: "task", Title: "Another task", Timezone: "UTC"},
	}, "UTC")
	if err != nil || len(created) != 2 {
		t.Fatalf("create fixtures: %#v %v", created, err)
	}
	var eventsBefore int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events").Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	action := AssistantAction{ActionID: "list", Operation: AssistantListItems, Payload: &AssistantListItemsPayload{Status: "todo"}}
	record, err := NewAssistantExecutor(store).Preview(AssistantPlanRequest{
		WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "list-items", Plan: assistantPlanFor(action),
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := NewAssistantExecutor(store).Execute(AssistantExecuteRequest{WorkspaceID: record.WorkspaceID, PlanID: record.ID})
	if err != nil || completed.State != AssistantPlanCompleted {
		t.Fatalf("execute list: %#v %v", completed, err)
	}
	if len(completed.Actions) != 1 || len(completed.Actions[0].Result) == 0 {
		t.Fatalf("missing list result: %#v", completed.Actions)
	}
	var result struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(completed.Actions[0].Result, &result); err != nil || result.Count != 2 {
		t.Fatalf("list result: %s %v", completed.Actions[0].Result, err)
	}
	items, err := store.listItems("todo", 100)
	if err != nil || len(items) != 2 || items[0].Version != 1 || items[1].Version != 1 {
		t.Fatalf("list mutated items: %#v %v", items, err)
	}
	var eventsAfter int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events").Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != eventsBefore {
		t.Fatalf("list wrote events: before=%d after=%d", eventsBefore, eventsAfter)
	}
}

func TestAssistantExecutorDuplicateSubmitIsIdempotent(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	req := AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "request-1", ConversationID: mustConversation(t, store), TurnID: "turn-1", Plan: assistantPlanFor(assistantCreateAction("once"))}
	preview, err := executor.Preview(req)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := executor.Preview(req)
	if err != nil || duplicate.ID != preview.ID {
		t.Fatalf("duplicate preview: %#v %v", duplicate, err)
	}
	first, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: req.WorkspaceID, PlanID: preview.ID, Confirmed: true})
	if err != nil || first.State != AssistantPlanCompleted {
		t.Fatalf("execute: %#v %v", first, err)
	}
	againPreview, err := executor.Preview(req)
	if err != nil || againPreview.ID != preview.ID || againPreview.State != AssistantPlanCompleted {
		t.Fatalf("retry preview after mutation: %#v %v", againPreview, err)
	}
	second, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: req.WorkspaceID, IdempotencyKey: req.IdempotencyKey, Confirmed: true})
	if err != nil || second.ID != first.ID || second.State != AssistantPlanCompleted {
		t.Fatalf("duplicate execute: %#v %v", second, err)
	}
	items, err := store.listItems("", 100)
	if err != nil || len(items) != 1 || items[0].Title != "once" {
		t.Fatalf("duplicate created item: %#v %v", items, err)
	}
	var events int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events WHERE item_id=?", items[0].ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("duplicate event count=%d err=%v", events, err)
	}
}

func TestAssistantExecutorUndoRestoresOnlyUnchangedItem(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	req := AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "undo-request", Plan: assistantPlanFor(assistantCreateAction("undo me"))}
	record, err := executor.Preview(req)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: req.WorkspaceID, PlanID: record.ID, Confirmed: true})
	if err != nil || completed.State != AssistantPlanCompleted {
		t.Fatalf("execute: %#v %v", completed, err)
	}
	undone, err := executor.Undo(req.WorkspaceID, record.ID)
	if err != nil || undone.ID != record.ID {
		t.Fatalf("undo: %#v %v", undone, err)
	}
	items, err := store.listItems("", 100)
	if err != nil || len(items) != 1 || items[0].Status != "archived" {
		t.Fatalf("created item was not reversibly archived: %#v %v", items, err)
	}
	if _, err := executor.Undo(req.WorkspaceID, record.ID); err == nil {
		t.Fatal("duplicate undo unexpectedly succeeded")
	}
}

func TestAssistantExecutorIdempotencyKeyCannotCrossConversationOrSourceRequest(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	firstConversation := mustConversation(t, store)
	secondConversation := mustConversation(t, store)
	plan := assistantPlanFor(assistantCreateAction("same plan"))
	first, err := executor.Preview(AssistantPlanRequest{
		WorkspaceID: store.defaultWorkspaceID(), ConversationID: firstConversation, IdempotencyKey: "shared-key",
		SourceRequestHash: "source-a", Plan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Preview(AssistantPlanRequest{
		WorkspaceID: store.defaultWorkspaceID(), ConversationID: secondConversation, IdempotencyKey: "shared-key",
		SourceRequestHash: "source-a", Plan: plan,
	}); err == nil || !strings.Contains(err.Error(), "idempotency key already used") {
		t.Fatalf("cross-conversation key was accepted: %v", err)
	}
	if _, err := executor.Preview(AssistantPlanRequest{
		WorkspaceID: store.defaultWorkspaceID(), ConversationID: firstConversation, IdempotencyKey: "shared-key",
		SourceRequestHash: "source-b", Plan: plan,
	}); err == nil || !strings.Contains(err.Error(), "idempotency key already used") {
		t.Fatalf("different source request reused key: %v", err)
	}
	replayed, err := executor.Preview(AssistantPlanRequest{
		WorkspaceID: store.defaultWorkspaceID(), ConversationID: firstConversation, IdempotencyKey: "shared-key",
		SourceRequestHash: "source-a", Plan: plan,
	})
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("matching source request did not replay: %#v %v", replayed, err)
	}
}

func mustConversation(t *testing.T, store *Store) string {
	t.Helper()
	c, err := store.createAssistantConversation()
	if err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func TestAssistantExecutorConfirmationExpiryAndCancellation(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	item := createContextItem(t, store, "danger", "", "", "")
	plan := assistantPlanFor(AssistantAction{ActionID: "archive", Operation: AssistantArchiveItem, Payload: &AssistantItemTargetPayload{ItemID: item.ID}})
	plan.RiskLevel = AssistantRiskHigh
	req := AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "archive-1", Plan: plan}
	record, err := executor.Preview(req)
	if err != nil || record.State != AssistantPlanAwaitingConfirmation {
		t.Fatalf("preview state: %#v %v", record, err)
	}
	if _, err := executor.Cancel(req.WorkspaceID, record.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.getAssistantPlan(req.WorkspaceID, record.ID)
	if err != nil || cancelled.State != AssistantPlanCancelled {
		t.Fatalf("cancel state: %#v %v", cancelled, err)
	}
	if _, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: req.WorkspaceID, PlanID: record.ID, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	current, err := store.getItem(item.ID)
	if err != nil || current.Status != "todo" {
		t.Fatalf("cancelled plan mutated item: %#v %v", current, err)
	}

	req.IdempotencyKey = "archive-expired"
	record, err = executor.Preview(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("UPDATE assistant_plans SET confirmation_expires_at='2000-01-01T00:00:00Z' WHERE id=?", record.ID); err != nil {
		t.Fatal(err)
	}
	expired, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: req.WorkspaceID, PlanID: record.ID, Confirmed: true})
	if err != nil || expired.State != AssistantPlanStale || !strings.Contains(expired.Error, "expired") {
		t.Fatalf("expired confirmation: %#v %v", expired, err)
	}
}

func TestAssistantExecutorStaleVersionAndCrossWorkspace(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	item := createContextItem(t, store, "stale", "", "", "")
	newTitle := "updated"
	expected := item.Version
	action := AssistantAction{ActionID: "update", Operation: AssistantUpdateItem, ExpectedItemVersion: &expected, Payload: &AssistantUpdateItemPayload{ItemID: item.ID, Changes: AssistantItemFields{Title: &newTitle}}}
	record, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "stale-1", Plan: assistantPlanFor(action)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.completeItem(item.ID); err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil || result.State != AssistantPlanStale || result.Actions[0].State != AssistantActionConflict {
		t.Fatalf("stale result: %#v %v", result, err)
	}
	if _, err := executor.Preview(AssistantPlanRequest{WorkspaceID: "workspace_missing", IdempotencyKey: "foreign", Plan: assistantPlanFor(assistantCreateAction("foreign"))}); err == nil {
		t.Fatal("missing workspace accepted")
	}
}

func TestAssistantExecutorPartialFailureAndAccurateSummary(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	item := createContextItem(t, store, "existing", "", "", "")
	other := createContextItem(t, store, "other", "", "", "")
	good := "changed"
	staleVersion := other.Version
	actions := []AssistantAction{
		{ActionID: "good", Operation: AssistantUpdateItem, Payload: &AssistantUpdateItemPayload{ItemID: item.ID, Changes: AssistantItemFields{Title: &good}}},
		{ActionID: "bad", Operation: AssistantUpdateItem, ExpectedItemVersion: &staleVersion, Payload: &AssistantUpdateItemPayload{ItemID: other.ID, Changes: AssistantItemFields{Title: &good}}},
	}
	plan := assistantPlanFor(actions...)
	record, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "partial", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.completeItem(other.ID); err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil || result.State != AssistantPlanPartiallyFailed || !strings.Contains(result.Error, "1 of 2") {
		t.Fatalf("partial summary: %#v %v", result, err)
	}
	if result.Actions[0].State != AssistantActionSucceeded || result.Actions[1].State != AssistantActionConflict {
		t.Fatalf("per action states: %#v", result.Actions)
	}
	updated, _ := store.getItem(item.ID)
	if updated.Title != good {
		t.Fatalf("successful action did not persist: %#v", updated)
	}
}

func TestAssistantExecutorMergeRollbackOnVersionConflict(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	source, err := store.createProject(Project{Name: "source"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.createProject(Project{Name: "target"})
	if err != nil {
		t.Fatal(err)
	}
	item := createContextItem(t, store, "owned", source.Name, "", "")
	expectedTarget := target.Version
	action := AssistantAction{ActionID: "merge", Operation: AssistantMergeProjects, Payload: &AssistantMergeProjectsPayload{SourceProjectID: source.ID, TargetProjectID: target.ID}, ExpectedProjectVersion: &expectedTarget}
	if err := action.Validate(); err != nil {
		t.Fatal(err)
	}
	record, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "merge-conflict", Plan: assistantPlanFor(action)})
	if err != nil {
		t.Fatal(err)
	}
	projectDescription := "changed"
	target.Description = projectDescription
	if _, err := store.updateProject(target); err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil || result.State != AssistantPlanStale {
		t.Fatalf("stale merge result: %#v %v", result, err)
	}
	// Validation fails before a transaction, leaving both projects and the item intact.
	if _, err := store.getProject(source.ID); err != nil {
		t.Fatalf("source removed after rejected merge: %v", err)
	}
	remaining, err := store.getItem(item.ID)
	if err != nil || remaining.ProjectID != source.ID {
		t.Fatalf("item moved after rejected merge: %#v %v", remaining, err)
	}
}

func TestAssistantExecutorConcurrentIdempotentPreview(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	req := AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "race-preview", Plan: assistantPlanFor(assistantCreateAction("race"))}
	const workers = 8
	results := make(chan AssistantPlanRecord, workers)
	errorsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, err := executor.Preview(req)
			if err != nil {
				errorsCh <- err
			} else {
				results <- record
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent preview failed: %v", err)
	}
	var first string
	for record := range results {
		if first == "" {
			first = record.ID
		}
		if record.ID != first {
			t.Fatalf("idempotent preview returned different IDs: %q vs %q", first, record.ID)
		}
	}
	var count int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM assistant_plans WHERE workspace_id=? AND idempotency_key=?", req.WorkspaceID, req.IdempotencyKey).Scan(&count); err != nil || count != 1 {
		t.Fatalf("plan rows=%d err=%v", count, err)
	}
}

func TestAssistantExecutorLeaseRecoveryAndInProgressResult(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	record, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "lease", Plan: assistantPlanFor(assistantCreateAction("lease"))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("UPDATE assistant_plans SET state=?, lease_until=?, lease_token=? WHERE id=?", AssistantPlanExecuting, "2999-01-01T00:00:00Z", "active-lease", record.ID); err != nil {
		t.Fatal(err)
	}
	inProgress, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil || inProgress.State != AssistantPlanExecuting {
		t.Fatalf("active lease was not respected: %#v %v", inProgress, err)
	}
	if _, err := store.db.Exec("UPDATE assistant_plans SET lease_until=? WHERE id=?", "2000-01-01T00:00:00Z", record.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil || recovered.State != AssistantPlanCompleted {
		t.Fatalf("expired lease was not recovered: %#v %v", recovered, err)
	}
}

func TestAssistantExecutorProjectAuditEventIDsAndReversibleMerge(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	name := "Audited"
	create := AssistantAction{ActionID: "project", Operation: AssistantCreateProject, Payload: &AssistantCreateProjectPayload{Project: AssistantProjectFields{Name: &name}}}
	record, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "project-audit", Plan: assistantPlanFor(create)})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil || result.State != AssistantPlanCompleted || len(result.Actions[0].EventIDs) != 1 {
		t.Fatalf("project create audit: %#v %v", result, err)
	}
	projects, err := store.listProjects()
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects: %#v %v", projects, err)
	}
	if events, eventErr := store.listProjectEvents(store.defaultWorkspaceID(), projects[0].ID, 10); eventErr != nil || len(events) != 1 || events[0].EventType != "created" {
		t.Fatalf("project event history: %#v %v", events, eventErr)
	}
	source, err := store.createProject(Project{Name: "Merge source"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.createProject(Project{Name: "Merge target"})
	if err != nil {
		t.Fatal(err)
	}
	item := createContextItem(t, store, "merge item", source.Name, "", "")
	mergeAction := AssistantAction{ActionID: "merge", Operation: AssistantMergeProjects, Payload: &AssistantMergeProjectsPayload{SourceProjectID: source.ID, TargetProjectID: target.ID}}
	mergeRecord, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "merge-audit", Plan: assistantPlanFor(mergeAction)})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: mergeRecord.ID, Confirmed: true})
	if err != nil || merged.State != AssistantPlanCompleted {
		t.Fatalf("merge execute: %#v %v", merged, err)
	}
	archived, err := store.getProject(source.ID)
	if err != nil || archived.Status != "archived" || archived.ArchivedFromStatus != "active" {
		t.Fatalf("source archive: %#v %v", archived, err)
	}
	if len(merged.Actions[0].EventIDs) < 2 {
		t.Fatalf("merge audit event IDs incomplete: %#v", merged.Actions[0].EventIDs)
	}
	if moved, err := store.getItem(item.ID); err != nil || moved.ProjectID != target.ID {
		t.Fatalf("merge membership: %#v %v", moved, err)
	}
}

func TestAssistantExecutorLeaseFencesMutationAfterActionClaim(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	record, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "fenced-create", Plan: assistantPlanFor(assistantCreateAction("must not duplicate"))})
	if err != nil {
		t.Fatal(err)
	}
	beforeAssistantActionHook = func() {
		_, _ = store.db.Exec("UPDATE assistant_plans SET lease_until='2000-01-01T00:00:00Z' WHERE id=?", record.ID)
	}
	t.Cleanup(func() { beforeAssistantActionHook = nil })
	result, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.State == AssistantPlanCompleted {
		t.Fatalf("expired lease claimed success: %#v", result)
	}
	items, err := store.listItems("", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Title == "must not duplicate" {
			t.Fatalf("expired worker mutated after fence: %#v", item)
		}
	}
}

func TestAssistantExecutorRecoveryDoesNotRerunActionAfterLeaseExpiresBeforeResult(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	record, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "fenced-result", Plan: assistantPlanFor(assistantCreateAction("one create"))})
	if err != nil {
		t.Fatal(err)
	}
	first := true
	afterAssistantActionMutationHook = func() {
		if first {
			first = false
			_, _ = store.db.Exec("UPDATE assistant_plans SET lease_until='2000-01-01T00:00:00Z' WHERE id=?", record.ID)
		}
	}
	t.Cleanup(func() { afterAssistantActionMutationHook = nil })
	_, err = executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.listItems("", 100)
	if err != nil {
		t.Fatal(err)
	}
	created := 0
	for _, item := range items {
		if item.Title == "one create" {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("initial mutation count=%d, want one", created)
	}
	recovered, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State == AssistantPlanCompleted {
		t.Fatalf("recovered expired action claimed success: %#v", recovered)
	}
	items, err = store.listItems("", 100)
	if err != nil {
		t.Fatal(err)
	}
	created = 0
	for _, item := range items {
		if item.Title == "one create" {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("recovery reran create: count=%d", created)
	}
}

func TestAssistantExecutorRejectsUnauthorizedWorkspaceAndExpiresExecutingConfirmation(t *testing.T) {
	store := testStore(t)
	executor := NewAssistantExecutor(store)
	if _, err := executor.Preview(AssistantPlanRequest{WorkspaceID: "workspace_other", IdempotencyKey: "unauthorized", Plan: assistantPlanFor(assistantCreateAction("nope"))}); err == nil {
		t.Fatal("unauthorized workspace accepted")
	}
	item := createContextItem(t, store, "confirm expiry", "", "", "")
	action := AssistantAction{ActionID: "archive", Operation: AssistantArchiveItem, Payload: &AssistantItemTargetPayload{ItemID: item.ID}}
	plan := assistantPlanFor(action)
	plan.RequiresConfirmation = true
	record, err := executor.Preview(AssistantPlanRequest{WorkspaceID: store.defaultWorkspaceID(), IdempotencyKey: "executing-expiry", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("UPDATE assistant_plans SET state=?, confirmation_expires_at='2000-01-01T00:00:00Z', lease_until='2999-01-01T00:00:00Z', lease_token='expired-worker' WHERE id=?", AssistantPlanExecuting, record.ID); err != nil {
		t.Fatal(err)
	}
	expired, err := executor.Execute(AssistantExecuteRequest{WorkspaceID: store.defaultWorkspaceID(), PlanID: record.ID, Confirmed: true})
	if err != nil || expired.State != AssistantPlanStale {
		t.Fatalf("executing confirmation did not expire: %#v %v", expired, err)
	}
}
