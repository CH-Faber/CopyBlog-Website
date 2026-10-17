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
INSERT INTO captures(id, source_type, raw_text, status, created_at, updated_at) VALUES('capture_legacy', 'text', 'legacy capture', 'received', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO items(id, capture_id, type, title, timezone, status, created_at, updated_at) VALUES('item_legacy', 'capture_legacy', 'task', 'legacy item', 'UTC', 'todo', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO projects(id, name, created_at, updated_at) VALUES('project_legacy', 'legacy project', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO memories(id, kind, content, created_at, updated_at) VALUES('memory_legacy', 'preference', 'legacy memory', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');`
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
	for _, table := range []string{"captures", "items", "projects", "memories"} {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE workspace_id=?", workspaceID).Scan(&count); err != nil {
			t.Fatalf("count migrated %s rows: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected one migrated %s row, got %d", table, count)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
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
}
