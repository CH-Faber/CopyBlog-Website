package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestLegacyDatabaseGetsDefaultWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := `
CREATE TABLE captures (id TEXT PRIMARY KEY, source_type TEXT NOT NULL, raw_text TEXT NOT NULL DEFAULT '', attachment_name TEXT NOT NULL DEFAULT '', attachment_mime TEXT NOT NULL DEFAULT '', attachment_path TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, ai_result TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE items (id TEXT PRIMARY KEY, capture_id TEXT, type TEXT NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', start_at TEXT NOT NULL DEFAULT '', end_at TEXT NOT NULL DEFAULT '', due_at TEXT NOT NULL DEFAULT '', reminder_at TEXT NOT NULL DEFAULT '', timezone TEXT NOT NULL, all_day INTEGER NOT NULL DEFAULT 0, recurrence_rule TEXT NOT NULL DEFAULT '', priority INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, project TEXT NOT NULL DEFAULT '', tags_json TEXT NOT NULL DEFAULT '[]', location TEXT NOT NULL DEFAULT '', people_json TEXT NOT NULL DEFAULT '[]', version INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE projects (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE, description TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active', priority INTEGER NOT NULL DEFAULT 0, color TEXT NOT NULL DEFAULT '', goal TEXT NOT NULL DEFAULT '', deadline TEXT NOT NULL DEFAULT '', review_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE memories (id TEXT PRIMARY KEY, kind TEXT NOT NULL, memory_key TEXT NOT NULL DEFAULT '', content TEXT NOT NULL, scope TEXT NOT NULL DEFAULT 'global', status TEXT NOT NULL DEFAULT 'proposed', evidence_count INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE reminders (id TEXT PRIMARY KEY, item_id TEXT NOT NULL, remind_at TEXT NOT NULL, status TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, sent_at TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE item_events (id TEXT PRIMARY KEY, item_id TEXT NOT NULL, event_type TEXT NOT NULL, data_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL);
INSERT INTO captures(id, source_type, raw_text, attachment_name, attachment_mime, attachment_path, status, created_at, updated_at) VALUES('capture_legacy', 'image', 'legacy capture', 'legacy.png', 'image/png', '/legacy/attachment.png', 'received', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO items(id, capture_id, type, title, timezone, status, created_at, updated_at) VALUES('item_legacy', 'capture_legacy', 'task', 'legacy item', 'UTC', 'todo', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO projects(id, name, created_at, updated_at) VALUES('project_legacy', 'legacy project', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO memories(id, kind, content, created_at, updated_at) VALUES('memory_legacy', 'preference', 'legacy memory', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO reminders(id, item_id, remind_at, status, created_at, updated_at) VALUES('reminder_legacy', 'item_legacy', '2026-01-01T08:00:00Z', 'pending', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO item_events(id, item_id, event_type, data_json, created_at) VALUES('event_legacy', 'item_legacy', 'created', '{"source":"legacy"}', '2026-01-01T00:00:00Z');`
	if _, err := db.Exec(legacySchema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := store.defaultWorkspaceID()
	if workspaceID == "" {
		t.Fatal("default workspace ID is empty")
	}
	for _, table := range []string{"captures", "items", "projects", "memories", "reminders", "item_events"} {
		query := "SELECT COUNT(*) FROM " + table
		if table == "reminders" || table == "item_events" {
			query += " WHERE item_id='item_legacy'"
		} else {
			query += " WHERE workspace_id=?"
		}
		var count int
		var err error
		if table == "reminders" || table == "item_events" {
			err = store.db.QueryRow(query).Scan(&count)
		} else {
			err = store.db.QueryRow(query, workspaceID).Scan(&count)
		}
		if err != nil {
			t.Fatalf("count migrated %s rows: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected one migrated %s row, got %d", table, count)
		}
	}
	var rawText, title, projectName, memoryContent, remindAt, eventData string
	if err := store.db.QueryRow("SELECT raw_text FROM captures WHERE id='capture_legacy' AND workspace_id=?", workspaceID).Scan(&rawText); err != nil || rawText != "legacy capture" {
		t.Fatalf("capture value lost: %q %v", rawText, err)
	}
	legacyCapture, err := store.getCapture("capture_legacy")
	if err != nil || len(legacyCapture.Attachments) != 1 || legacyCapture.Attachments[0].Name != "legacy.png" {
		t.Fatalf("legacy attachment was not migrated: %#v %v", legacyCapture.Attachments, err)
	}
	if err := store.db.QueryRow("SELECT title FROM items WHERE id='item_legacy' AND workspace_id=?", workspaceID).Scan(&title); err != nil || title != "legacy item" {
		t.Fatalf("item value lost: %q %v", title, err)
	}
	if err := store.db.QueryRow("SELECT name FROM projects WHERE id='project_legacy' AND workspace_id=?", workspaceID).Scan(&projectName); err != nil || projectName != "legacy project" {
		t.Fatalf("project value lost: %q %v", projectName, err)
	}
	var projectVersion int
	if err := store.db.QueryRow("SELECT version FROM projects WHERE id='project_legacy' AND workspace_id=?", workspaceID).Scan(&projectVersion); err != nil || projectVersion != 1 {
		t.Fatalf("legacy project version = %d, err=%v; want 1", projectVersion, err)
	}
	if err := store.db.QueryRow("SELECT content FROM memories WHERE id='memory_legacy' AND workspace_id=?", workspaceID).Scan(&memoryContent); err != nil || memoryContent != "legacy memory" {
		t.Fatalf("memory value lost: %q %v", memoryContent, err)
	}
	if err := store.db.QueryRow("SELECT remind_at FROM reminders WHERE id='reminder_legacy'").Scan(&remindAt); err != nil || remindAt != "2026-01-01T08:00:00Z" {
		t.Fatalf("reminder value lost: %q %v", remindAt, err)
	}
	if err := store.db.QueryRow("SELECT data_json FROM item_events WHERE id='event_legacy'").Scan(&eventData); err != nil || eventData != "{\"source\":\"legacy\"}" {
		t.Fatalf("event value lost: %q %v", eventData, err)
	}
	var linked int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM reminders r JOIN items i ON i.id=r.item_id WHERE r.id='reminder_legacy' AND i.workspace_id=?", workspaceID).Scan(&linked); err != nil || linked != 1 {
		t.Fatalf("reminder workspace ownership was not preserved: %d %v", linked, err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM item_events e JOIN items i ON i.id=e.item_id WHERE e.id='event_legacy' AND i.workspace_id=?", workspaceID).Scan(&linked); err != nil || linked != 1 {
		t.Fatalf("item event workspace ownership was not preserved: %d %v", linked, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, table := range []string{"captures", "items", "projects", "memories"} {
		var count int
		if err := reopened.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE workspace_id=?", workspaceID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("reopen changed migrated %s rows: %d %v", table, count, err)
		}
	}
	if err := reopened.db.QueryRow("SELECT COUNT(*) FROM reminders r JOIN items i ON i.id=r.item_id WHERE r.id='reminder_legacy' AND i.workspace_id=?", workspaceID).Scan(&linked); err != nil || linked != 1 {
		t.Fatalf("reopen lost reminder workspace link: %d %v", linked, err)
	}
	if err := reopened.db.QueryRow("SELECT COUNT(*) FROM item_events e JOIN items i ON i.id=e.item_id WHERE e.id='event_legacy' AND i.workspace_id=?", workspaceID).Scan(&linked); err != nil || linked != 1 {
		t.Fatalf("reopen lost item event workspace link: %d %v", linked, err)
	}
	if err := reopened.db.QueryRow("SELECT raw_text FROM captures WHERE id='capture_legacy' AND workspace_id=?", workspaceID).Scan(&rawText); err != nil || rawText != "legacy capture" {
		t.Fatalf("reopen changed capture value: %q %v", rawText, err)
	}
	if err := reopened.db.QueryRow("SELECT title FROM items WHERE id='item_legacy' AND workspace_id=?", workspaceID).Scan(&title); err != nil || title != "legacy item" {
		t.Fatalf("reopen changed item value: %q %v", title, err)
	}
	if err := reopened.db.QueryRow("SELECT name FROM projects WHERE id='project_legacy' AND workspace_id=?", workspaceID).Scan(&projectName); err != nil || projectName != "legacy project" {
		t.Fatalf("reopen changed project value: %q %v", projectName, err)
	}
	if err := reopened.db.QueryRow("SELECT content FROM memories WHERE id='memory_legacy' AND workspace_id=?", workspaceID).Scan(&memoryContent); err != nil || memoryContent != "legacy memory" {
		t.Fatalf("reopen changed memory value: %q %v", memoryContent, err)
	}
	if err := reopened.db.QueryRow("SELECT remind_at FROM reminders WHERE id='reminder_legacy'").Scan(&remindAt); err != nil || remindAt != "2026-01-01T08:00:00Z" {
		t.Fatalf("reopen changed reminder value: %q %v", remindAt, err)
	}
	if err := reopened.db.QueryRow("SELECT data_json FROM item_events WHERE id='event_legacy'").Scan(&eventData); err != nil || eventData != "{\"source\":\"legacy\"}" {
		t.Fatalf("reopen changed event value: %q %v", eventData, err)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agenda.db")
	first, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := first.defaultWorkspaceID()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.defaultWorkspaceID() != workspaceID {
		t.Fatalf("default workspace changed from %q to %q", workspaceID, second.defaultWorkspaceID())
	}
	var workspaces, users, memberships, versions int
	if err := second.db.QueryRow("SELECT COUNT(*) FROM workspaces").Scan(&workspaces); err != nil {
		t.Fatal(err)
	}
	if err := second.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := second.db.QueryRow("SELECT COUNT(*) FROM workspace_members").Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if err := second.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=1").Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if workspaces != 1 || users != 1 || memberships != 1 || versions != 1 {
		t.Fatalf("migration duplicated deterministic records: workspaces=%d users=%d memberships=%d versions=%d", workspaces, users, memberships, versions)
	}
	if err := second.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=3").Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("assistant migration was not recorded exactly once: count=%d err=%v", versions, err)
	}
	if err := second.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=4").Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("project version migration was not recorded exactly once: count=%d err=%v", versions, err)
	}
	project, err := second.createProject(Project{Name: "Versioned"})
	if err != nil || project.Version != 1 {
		t.Fatalf("new project version = %d, err=%v; want 1", project.Version, err)
	}
	for _, table := range []string{"assistant_conversations", "assistant_turns"} {
		var count int
		if err := second.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("assistant migration did not create %s: count=%d err=%v", table, count, err)
		}
	}
}
