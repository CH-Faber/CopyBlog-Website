package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func createContextItem(t *testing.T, store *Store, title, project string, startAt, dueAt string) Item {
	t.Helper()
	candidate := Candidate{Title: title, Type: "task", Project: project, StartAt: startAt, DueAt: dueAt}
	items, err := store.createItems("", []Candidate{candidate}, "UTC")
	if err != nil || len(items) != 1 {
		t.Fatalf("create item %q: items=%#v err=%v", title, items, err)
	}
	return items[0]
}

func TestAssistantTargetResolutionExactTitleAndDuplicateClarification(t *testing.T) {
	store := testStore(t)
	item := createContextItem(t, store, "Prepare report", "", "2026-09-27T10:00:00Z", "")
	resolved, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), "Prepare report", "", "", "UTC", time.Time{})
	if err != nil || !resolved.Resolved || resolved.Target == nil || resolved.Target.ID != item.ID {
		t.Fatalf("exact title did not resolve: %#v err=%v", resolved, err)
	}

	other := createContextItem(t, store, "Prepare report", "", "2026-09-28T10:00:00Z", "")
	ambiguous, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), "Prepare report", "", "", "UTC", time.Time{})
	if err != nil || ambiguous.Resolved || !ambiguous.NeedsChoice || len(ambiguous.Candidates) != 2 {
		t.Fatalf("duplicate title did not request clarification: %#v err=%v", ambiguous, err)
	}
	if ambiguous.Candidates[0].ID == ambiguous.Candidates[1].ID || (ambiguous.Candidates[0].ID != other.ID && ambiguous.Candidates[1].ID != other.ID) {
		t.Fatalf("candidate list is incomplete or duplicated: %#v", ambiguous.Candidates)
	}
}

func TestAssistantTargetResolutionIsWorkspaceAndProjectScoped(t *testing.T) {
	store := testStore(t)
	first := createContextItem(t, store, "Review", "Alpha", "", "")
	second := createContextItem(t, store, "Review", "Beta", "", "")
	filtered, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), "Review", "Beta", "", "UTC", time.Time{})
	if err != nil || !filtered.Resolved || filtered.Target.ID != second.ID {
		t.Fatalf("project scope did not narrow title: %#v err=%v", filtered, err)
	}
	foreignWorkspace := "workspace_foreign_context"
	if _, err := store.db.Exec(`INSERT INTO workspaces(id, owner_user_id, name, kind, created_at, updated_at) VALUES(?, ?, 'Foreign', 'personal', ?, ?)`, foreignWorkspace, defaultPersonalUserID, nowString(), nowString()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO items(id, workspace_id, type, title, timezone, status, project, created_at, updated_at) VALUES('foreign_review', ?, 'task', 'Review', 'UTC', 'todo', '', ?, ?)`, foreignWorkspace, nowString(), nowString()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO projects(id, workspace_id, name, created_at, updated_at) VALUES('foreign_only_project', ?, 'ForeignOnly', ?, ?)`, foreignWorkspace, nowString(), nowString()); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), "Review", "Alpha", "", "UTC", time.Time{})
	if err != nil || !resolved.Resolved || resolved.Target.ID != first.ID {
		t.Fatalf("foreign workspace or other project leaked into target resolution: %#v err=%v", resolved, err)
	}
	missingProject, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), "Review", "Unknown", "", "UTC", time.Time{})
	if err != nil || !missingProject.NeedsChoice || missingProject.Resolved {
		t.Fatalf("unknown project should not resolve: %#v err=%v", missingProject, err)
	}
	foreignProject, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), "Review", "ForeignOnly", "", "UTC", time.Time{})
	if err != nil || !foreignProject.NeedsChoice || foreignProject.Resolved {
		t.Fatalf("foreign-workspace project should not resolve in the default workspace: %#v err=%v", foreignProject, err)
	}
}

func TestAssistantTargetResolutionIncludesCompletedAndArchivedItems(t *testing.T) {
	store := testStore(t)
	completed := createContextItem(t, store, "Finished", "", "", "")
	completed, err := store.completeItem(completed.ID)
	if err != nil || completed.Status != "done" {
		t.Fatalf("complete item: %#v err=%v", completed, err)
	}
	archived := createContextItem(t, store, "Filed", "", "", "")
	archived, err = store.archiveItem(archived.ID)
	if err != nil || archived.Status != "archived" {
		t.Fatalf("archive item: %#v err=%v", archived, err)
	}
	for title, want := range map[string]string{"Finished": "done", "Filed": "archived"} {
		result, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), title, "", "", "UTC", time.Time{})
		if err != nil || !result.Resolved || result.Target.Status != want {
			t.Errorf("%s target status: %#v err=%v", title, result, err)
		}
	}
}

func TestAssistantTargetResolutionDatePhraseUsesHalfOpenLocalDay(t *testing.T) {
	store := testStore(t)
	inside := createContextItem(t, store, "Pay rent", "", "2026-09-27T15:59:59Z", "")
	createContextItem(t, store, "Pay rent", "", "2026-09-27T16:00:00Z", "")
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	result, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), "Pay rent", "", "today", "Asia/Shanghai", now)
	if err != nil || !result.Resolved || result.Target.ID != inside.ID {
		t.Fatalf("local day boundary should include start and exclude next day: %#v err=%v", result, err)
	}
}

func TestAssistantTargetResolutionEmptyResultsAskForClarification(t *testing.T) {
	store := testStore(t)
	result, err := store.resolveAssistantItemTarget(store.defaultWorkspaceID(), "Missing", "", "", "UTC", time.Time{})
	if err != nil || result.Resolved || !result.NeedsChoice || len(result.Candidates) != 0 {
		t.Fatalf("empty result should request clarification: %#v err=%v", result, err)
	}
}

func TestAssistantContextIsBoundedScopedAndCompact(t *testing.T) {
	store := testStore(t)
	item := createContextItem(t, store, "Context item", "Context project", "", "")
	if _, err := store.db.Exec("UPDATE items SET description='private description', location='private location', people_json='[\"private person\"]' WHERE id=? AND workspace_id=?", item.ID, store.defaultWorkspaceID()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.createMemory(Memory{Content: "  remember   this  ", Kind: "preference", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	ctx, err := store.assistantContext(store.defaultWorkspaceID(), AssistantContextOptions{ItemLimit: 100000, ProjectLimit: 100000, EventLimit: 100000, MemoryLimit: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Items) != 1 || ctx.Items[0].Title != "Context item" || ctx.Items[0].Project != "Context project" {
		t.Fatalf("unexpected compact items: %#v", ctx.Items)
	}
	if len(ctx.Memories) != 1 || ctx.Memories[0].Content != "remember this" {
		t.Fatalf("memory was not compacted: %#v", ctx.Memories)
	}
	encoded, err := json.Marshal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private") {
		t.Fatalf("private item data leaked into assistant context: %s", encoded)
	}
	if _, err := store.assistantContext("", AssistantContextOptions{}); err == nil {
		t.Fatal("missing workspace was accepted")
	}
}
