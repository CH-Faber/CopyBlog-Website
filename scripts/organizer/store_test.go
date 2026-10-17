package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "organizer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestAssistantConversationLifecycleRetentionAndWorkspaceIsolation(t *testing.T) {
	store := testStore(t)
	conversation, err := store.createAssistantConversationWithMetadata("test-provider", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.appendUserTurn(conversation.ID, "Keep my request verbatim")
	if err != nil || user.Role != "user" || user.Content != "Keep my request verbatim" || len(user.StructuredResult) != 0 {
		t.Fatalf("user turn was not persisted separately: %#v %v", user, err)
	}
	result := json.RawMessage(`{"itemIds":["item_example"],"changed":true}`)
	assistant, err := store.appendAssistantTurn(conversation.ID, "I added the task.", result, "", "test-provider", "test-model")
	if err != nil || assistant.Role != "assistant" || assistant.Content != "I added the task." || string(assistant.StructuredResult) != string(result) {
		t.Fatalf("assistant result was not persisted separately: %#v %v", assistant, err)
	}
	action, err := store.appendActionResultTurn(conversation.ID, "Task creation completed", json.RawMessage(`{"success":true}`), "", "", "")
	if err != nil || action.Role != "action" {
		t.Fatalf("action result turn was not stored: %#v %v", action, err)
	}
	turns, err := store.listAssistantTurns(conversation.ID)
	if err != nil || len(turns) != 3 || turns[0].Content != user.Content || turns[1].Content != assistant.Content || string(turns[1].StructuredResult) != string(result) || turns[2].Role != "action" {
		t.Fatalf("unexpected conversation turns: %#v %v", turns, err)
	}
	loaded, err := store.getAssistantConversation(conversation.ID)
	if err != nil || loaded.WorkspaceID != store.defaultWorkspaceID() || loaded.Title != "Keep my request verbatim" || loaded.Provider != "test-provider" || loaded.Model != "test-model" {
		t.Fatalf("conversation metadata mismatch: %#v %v", loaded, err)
	}
	listed, err := store.listAssistantConversations(10)
	if err != nil || len(listed) != 1 || listed[0].ID != conversation.ID {
		t.Fatalf("conversation was not listed: %#v %v", listed, err)
	}
	if _, err := store.appendUserTurn(conversation.ID, strings.Repeat("x", maxAssistantTurnText+1)); err == nil {
		t.Fatal("oversized user turn was accepted")
	}
	if _, err := store.appendAssistantTurn(conversation.ID, "bad result", json.RawMessage(`not json`), "", "", ""); err == nil {
		t.Fatal("invalid structured result was accepted")
	}
	const additionalTurns = 201
	for i := 0; i < additionalTurns; i++ {
		if _, err := store.appendAssistantTurn(conversation.ID, "retained response", nil, "", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	turns, err = store.listAssistantTurns(conversation.ID)
	if err != nil || len(turns) != additionalTurns+3 {
		t.Fatalf("durable turn history was truncated: count=%d err=%v", len(turns), err)
	}
	if _, err := store.db.Exec("INSERT INTO workspaces(id, owner_user_id, name, kind, created_at, updated_at) VALUES('workspace_other', ?, 'Other', 'personal', ?, ?)", defaultPersonalUserID, nowString(), nowString()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO assistant_conversations(id, workspace_id, status, created_at, updated_at)
VALUES('conversation_foreign', 'workspace_other', 'active', ?, ?)`, nowString(), nowString()); err != nil {
		t.Fatal(err)
	}
	listed, err = store.listAssistantConversations(10)
	if err != nil || len(listed) != 1 || listed[0].ID != conversation.ID {
		t.Fatalf("conversation list leaked another workspace: %#v %v", listed, err)
	}
	if _, err := store.getAssistantConversation("conversation_foreign"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign conversation was visible: %v", err)
	}
	if _, err := store.appendUserTurn("conversation_foreign", "cross-workspace"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign conversation accepted an appended turn: %v", err)
	}
	if err := store.deleteAssistantConversation("conversation_foreign"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign conversation was deletable: %v", err)
	}
	if err := store.deleteAssistantConversation(conversation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.getAssistantConversation(conversation.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted conversation remains: %v", err)
	}
	var foreignRemaining int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM assistant_conversations WHERE id='conversation_foreign' AND workspace_id='workspace_other'").Scan(&foreignRemaining); err != nil || foreignRemaining != 1 {
		t.Fatalf("foreign conversation changed by scoped delete: count=%d err=%v", foreignRemaining, err)
	}
}

func TestAssistantConversationSurvivesStoreReopenAndDeletePreservesItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assistant.db")
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.createItems("", []Candidate{{Type: "task", Title: "Independent Agenda item"}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.createAssistantConversation()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.appendUserTurn(conversation.ID, "Create the task"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.appendActionResultTurn(conversation.ID, "Created Agenda item", json.RawMessage(`{"itemId":"`+items[0].ID+`"}`), items[0].ID, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	turns, err := reopened.listAssistantTurns(conversation.ID)
	if err != nil || len(turns) != 2 || turns[0].Content != "Create the task" || turns[1].StructuredResultRef != items[0].ID {
		t.Fatalf("conversation did not survive reopening: %#v %v", turns, err)
	}
	if err := reopened.deleteAssistantConversation(conversation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.getItem(items[0].ID); err != nil {
		t.Fatalf("deleting conversation removed its Agenda item: %v", err)
	}
}

func TestAssistantConversationHistoryIsNotSilentlyPruned(t *testing.T) {
	store := testStore(t)
	const conversationCount = 105
	conversations := make([]AssistantConversation, conversationCount)
	for i := range conversations {
		conversation, err := store.createAssistantConversation()
		if err != nil {
			t.Fatal(err)
		}
		conversations[i] = conversation
	}
	oldest := conversations[0]
	if _, err := store.appendUserTurn(oldest.ID, "This history must remain available"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM assistant_conversations WHERE workspace_id=?", store.defaultWorkspaceID()).Scan(&count); err != nil || count != conversationCount {
		t.Fatalf("conversation history was pruned: count=%d err=%v", count, err)
	}
	turns, err := store.listAssistantTurns(oldest.ID)
	if err != nil || len(turns) != 1 || turns[0].Content != "This history must remain available" {
		t.Fatalf("old conversation history was lost: %#v %v", turns, err)
	}
}

func TestCaptureToItemLifecycle(t *testing.T) {
	store := testStore(t)
	capture, err := store.createCapture(Capture{SourceType: "text", RawText: "明天交报告"})
	if err != nil {
		t.Fatal(err)
	}
	result := ParseResult{Items: []Candidate{{Type: "task", Title: "提交报告", DueAt: "2026-09-22T18:00:00+08:00", ReminderAt: "2026-09-22T17:30:00+08:00", Timezone: "Asia/Shanghai"}}}
	if err := store.setCaptureResult(capture.ID, result); err != nil {
		t.Fatal(err)
	}
	items, err := store.createItems(capture.ID, result.Items, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "提交报告" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if items[0].DueAt != "2026-09-22T10:00:00Z" {
		t.Fatalf("time was not normalized: %s", items[0].DueAt)
	}
	confirmed, err := store.getCapture(capture.ID)
	if err != nil || confirmed.Status != "confirmed" {
		t.Fatalf("capture not confirmed: %#v, %v", confirmed, err)
	}
	completed, err := store.completeItem(items[0].ID)
	if err != nil || completed.Status != "done" || completed.Version != 2 {
		t.Fatalf("item not completed: %#v, %v", completed, err)
	}
}

func TestDirectItemDoesNotRequireCapture(t *testing.T) {
	store := testStore(t)
	items, err := store.createItems("", []Candidate{{Type: "task", Title: "独立事项"}}, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].CaptureID != "" {
		t.Fatalf("unexpected item: %#v", items)
	}
}

func TestItemHistoryProjectAndRestore(t *testing.T) {
	store := testStore(t)
	items, err := store.createItems("", []Candidate{{Type: "task", Title: "完成项目首页", Project: "网站改版", Certainty: "tentative", DurationMinutes: 90}}, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]
	if item.ProjectID == "" || item.Certainty != "tentative" || item.DurationMinutes != 90 {
		t.Fatalf("extended item fields were not stored: %#v", item)
	}
	projects, err := store.listProjects()
	if err != nil || len(projects) != 1 || projects[0].OpenCount != 1 {
		t.Fatalf("project was not created: %#v %v", projects, err)
	}
	completed, err := store.completeItem(item.ID)
	if err != nil || completed.Status != "done" || completed.CompletedAt == "" {
		t.Fatalf("completion was not retained: %#v %v", completed, err)
	}
	reopened, err := store.reopenItem(item.ID)
	if err != nil || reopened.Status != "todo" || reopened.CompletedAt != "" {
		t.Fatalf("item was not restored: %#v %v", reopened, err)
	}
	events, err := store.listItemEvents(item.ID, 20)
	if err != nil || len(events) != 3 || events[0].EventType != "reopened" {
		t.Fatalf("unexpected history: %#v %v", events, err)
	}
}

func TestMemoryApprovalAndPrompt(t *testing.T) {
	store := testStore(t)
	memory, err := store.createMemory(Memory{Kind: "preference", Content: "早上默认安排在 08:30。", Status: "active"})
	if err != nil || memory.ID == "" {
		t.Fatalf("create memory failed: %#v %v", memory, err)
	}
	prompt, err := store.activeMemoryPrompt()
	if err != nil || !strings.Contains(prompt, "08:30") {
		t.Fatalf("memory was not available to the assistant: %q %v", prompt, err)
	}
	memory.Status = "forgotten"
	if _, err := store.updateMemory(memory); err != nil {
		t.Fatal(err)
	}
	prompt, err = store.activeMemoryPrompt()
	if err != nil || prompt != "" {
		t.Fatalf("forgotten memory remained active: %q %v", prompt, err)
	}
}

func TestItemMutationsIncrementVersionAndRecordEvent(t *testing.T) {
	store := testStore(t)
	items, err := store.createItems("", []Candidate{{Type: "task", Title: "Versioned"}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	id := items[0].ID
	var initialEvents int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events WHERE item_id=? AND event_type='created'", id).Scan(&initialEvents); err != nil || items[0].Version != 1 || initialEvents != 1 {
		t.Fatalf("creation must start at version 1 with one event: version=%d events=%d err=%v", items[0].Version, initialEvents, err)
	}
	assertMutation := func(label string, mutate func() (Item, error)) Item {
		t.Helper()
		before, err := store.getItem(id)
		if err != nil {
			t.Fatal(err)
		}
		var eventCount int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events WHERE item_id=?", id).Scan(&eventCount); err != nil {
			t.Fatal(err)
		}
		after, err := mutate()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		var afterEventCount int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events WHERE item_id=?", id).Scan(&afterEventCount); err != nil {
			t.Fatal(err)
		}
		if after.Version != before.Version+1 || afterEventCount != eventCount+1 {
			t.Fatalf("%s did not create one versioned event: before=%d after=%d events=%d->%d", label, before.Version, after.Version, eventCount, afterEventCount)
		}
		var eventVersion int
		if err := store.db.QueryRow("SELECT COALESCE(json_extract(data_json, '$.version'), json_extract(data_json, '$.after.version')) FROM item_events WHERE item_id=? ORDER BY rowid DESC LIMIT 1", id).Scan(&eventVersion); err != nil || eventVersion != after.Version {
			t.Fatalf("%s event/version not committed together: event version=%d, item version=%d, err=%v", label, eventVersion, after.Version, err)
		}
		return after
	}

	assertMutation("update", func() (Item, error) {
		item, err := store.getItem(id)
		if err != nil {
			return Item{}, err
		}
		item.Title = "Updated"
		return store.updateItem(item)
	})
	assertMutation("snooze", func() (Item, error) { return store.snoozeItem(id, "2026-10-01T08:00:00Z") })
	assertMutation("complete", func() (Item, error) { return store.completeItem(id) })
	assertMutation("reopen", func() (Item, error) { return store.reopenItem(id) })
	archived := assertMutation("archive", func() (Item, error) { return store.archiveItem(id) })
	if archived.Status != "archived" || archived.ArchivedFromStatus != "todo" {
		t.Fatalf("archive did not retain reversible state: %#v", archived)
	}
	restored := assertMutation("restore archive", func() (Item, error) { return store.reopenItem(id) })
	if restored.Status != "todo" || restored.ArchivedFromStatus != "" {
		t.Fatalf("archive restore did not recover prior state: %#v", restored)
	}
}

func TestArchiveRestorePreservesLifecycleTimestamps(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Store, string) (Item, error)
		want string
	}{
		{name: "completed", set: (*Store).completeItem, want: "done"},
		{name: "cancelled", set: (*Store).cancelItem, want: "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := testStore(t)
			items, err := store.createItems("", []Candidate{{Type: "task", Title: "Lifecycle"}}, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			id := items[0].ID
			finished, err := tc.set(store, id)
			if err != nil {
				t.Fatal(err)
			}
			archived, err := store.archiveItem(id)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := store.reopenItem(id)
			if err != nil {
				t.Fatal(err)
			}
			if restored.Status != tc.want || restored.CompletedAt != finished.CompletedAt || restored.CancelledAt != finished.CancelledAt {
				t.Fatalf("archive/restore changed lifecycle state: before=%#v archived=%#v restored=%#v", finished, archived, restored)
			}
		})
	}
}

func TestProjectMembershipIsWorkspaceScoped(t *testing.T) {
	store := testStore(t)
	project, err := store.createProject(Project{Name: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.createItems("", []Candidate{{Type: "task", Title: "Membership", Project: project.Name}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	original, err := store.getItem(items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	project.Name = "Renamed"
	if _, err := store.updateProject(project); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.getItem(original.ID)
	if err != nil || renamed.Project != "Renamed" || renamed.Version != original.Version+1 {
		t.Fatalf("project rename did not version its member item: %#v %v", renamed, err)
	}
	var renameEvents int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events WHERE item_id=? AND event_type='project_renamed'", original.ID).Scan(&renameEvents); err != nil || renameEvents != 1 {
		t.Fatalf("project rename did not record one item event: count=%d err=%v", renameEvents, err)
	}
	destination, err := store.createProject(Project{Name: "Destination"})
	if err != nil {
		t.Fatal(err)
	}
	move := renamed
	move.Project = destination.Name
	moved, err := store.updateItem(move)
	if err != nil {
		t.Fatal(err)
	}
	var eventProjectID string
	if err := store.db.QueryRow("SELECT json_extract(data_json, '$.after.projectId') FROM item_events WHERE item_id=? ORDER BY rowid DESC LIMIT 1", moved.ID).Scan(&eventProjectID); err != nil || eventProjectID != moved.ProjectID || moved.ProjectID != destination.ID {
		t.Fatalf("membership event has stale project ID: snapshot=%q returned=%q destination=%q err=%v", eventProjectID, moved.ProjectID, destination.ID, err)
	}
	foreignProjectID := "project_other_workspace"
	if _, err := store.db.Exec(`INSERT INTO projects(id, workspace_id, name, created_at, updated_at) VALUES(?, 'workspace_other', 'Foreign', ?, ?)`, foreignProjectID, nowString(), nowString()); err != nil {
		t.Fatal(err)
	}
	item, err := store.getItem(items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	item.ProjectID = foreignProjectID
	if _, err := store.updateItem(item); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign-workspace project was accepted: %v", err)
	}
	unchanged, err := store.getItem(item.ID)
	if err != nil || unchanged.ProjectID != destination.ID || unchanged.Version != item.Version {
		t.Fatalf("failed foreign membership mutated item: %#v %v", unchanged, err)
	}
}

func TestProjectUpdateUsesVersionCompareAndSwap(t *testing.T) {
	store := testStore(t)
	project, err := store.createProject(Project{Name: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	if project.Version != 1 {
		t.Fatalf("created project version = %d, want 1", project.Version)
	}
	omitted := project
	omitted.Version = 0
	omitted.Name = "Omitted version"
	if _, err := store.updateProject(omitted); !errors.Is(err, errConflict) {
		t.Fatalf("update without expected version returned %v, want conflict", err)
	}
	update := project
	update.Name = "Updated"
	updated, err := store.updateProject(update)
	if err != nil || updated.Version != 2 {
		t.Fatalf("updated project version = %d, err=%v; want 2", updated.Version, err)
	}
	stale := project
	stale.Name = "Stale update"
	if _, err := store.updateProject(stale); !errors.Is(err, errConflict) {
		t.Fatalf("stale project update returned %v, want conflict", err)
	}
	unchanged, err := store.getProject(project.ID)
	if err != nil || unchanged.Name != "Updated" || unchanged.Version != 2 {
		t.Fatalf("stale update mutated project: %#v, err=%v", unchanged, err)
	}
	version, err := store.AssistantProjectVersion(project.WorkspaceID, project.ID)
	if err != nil || version != 2 {
		t.Fatalf("assistant project version = %d, err=%v; want 2", version, err)
	}
}

func TestProjectMergeRollsBackOnVersionConflict(t *testing.T) {
	store := testStore(t)
	source, err := store.createProject(Project{Name: "Source"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.createProject(Project{Name: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.createItems("", []Candidate{{Type: "task", Title: "First", Project: source.Name}, {Type: "task", Title: "Second", Project: source.Name}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]int{items[0].ID: items[0].Version, items[1].ID: items[1].Version + 1}
	if err := store.mergeProject(source.ID, target.ID, expected, source.Version+1, target.Version); !errors.Is(err, errConflict) {
		t.Fatalf("merge with stale source project version returned %v", err)
	}
	if err := store.mergeProject(source.ID, target.ID, expected, source.Version, target.Version+1); !errors.Is(err, errConflict) {
		t.Fatalf("merge with stale target project version returned %v", err)
	}
	if err := store.mergeProject(source.ID, target.ID, expected, source.Version, target.Version); !errors.Is(err, errConflict) {
		t.Fatalf("merge with stale item version returned %v", err)
	}
	if _, err := store.getProject(source.ID); err != nil {
		t.Fatalf("source project was deleted despite conflict: %v", err)
	}
	for _, original := range items {
		current, err := store.getItem(original.ID)
		if err != nil || current.ProjectID != source.ID || current.Version != original.Version {
			t.Fatalf("merge conflict partially mutated item: %#v %v", current, err)
		}
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events WHERE item_id=? AND event_type='project_merged'", original.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("merge conflict wrote an event for %s: count=%d err=%v", original.ID, count, err)
		}
	}
	expected[items[1].ID] = items[1].Version
	if err := store.mergeProject(source.ID, target.ID, expected, source.Version, target.Version); err != nil {
		t.Fatalf("merge with current item versions failed: %v", err)
	}
	mergedTarget, err := store.getProject(target.ID)
	if err != nil || mergedTarget.Version != target.Version+1 {
		t.Fatalf("merge did not advance target project version: %#v err=%v", mergedTarget, err)
	}
	archivedSource, err := store.getProject(source.ID)
	if err != nil || archivedSource.Status != "archived" || archivedSource.ArchivedFromStatus != "active" {
		t.Fatalf("source project was not reversibly archived: %#v %v", archivedSource, err)
	}
	for _, original := range items {
		current, err := store.getItem(original.ID)
		if err != nil || current.ProjectID != target.ID || current.Project != target.Name || current.Version != original.Version+1 {
			t.Fatalf("successful merge did not update membership once: %#v %v", current, err)
		}
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events WHERE item_id=? AND event_type='project_merged'", original.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("successful merge did not write one event: count=%d err=%v", count, err)
		}
	}
}
