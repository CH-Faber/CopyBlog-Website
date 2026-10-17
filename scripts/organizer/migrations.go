package main

import (
	"database/sql"
	"fmt"
)

const (
	defaultPersonalUserID      = "user_personal"
	defaultPersonalWorkspaceID = "workspace_personal"
)

type migration struct {
	version int
	apply   func(*sql.Tx) error
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
)`); err != nil {
		return err
	}

	migrations := []migration{{version: 1, apply: migrateV1}, {version: 2, apply: migrateV2}, {version: 3, apply: migrateV3}, {version: 4, apply: migrateV4}, {version: 5, apply: migrateV5}, {version: 6, apply: migrateV6}, {version: 7, apply: migrateV7}, {version: 8, apply: migrateV8}, {version: 9, apply: migrateV9}, {version: 10, apply: migrateV10}, {version: 11, apply: migrateV11}}
	for _, migration := range migrations {
		var applied int
		err := s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=?", migration.version).Scan(&applied)
		if err != nil {
			return err
		}
		if applied != 0 {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if err := migration.apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", migration.version, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)", migration.version, nowString()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", migration.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.version, err)
		}
	}
	return nil
}

func migrateV10(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE capture_attachments (
  capture_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  name TEXT NOT NULL,
  mime TEXT NOT NULL,
  path TEXT NOT NULL,
  PRIMARY KEY(capture_id, ordinal),
  FOREIGN KEY(capture_id) REFERENCES captures(id) ON DELETE CASCADE
);
INSERT INTO capture_attachments(capture_id, ordinal, name, mime, path)
SELECT id, 0, attachment_name, attachment_mime, attachment_path
FROM captures WHERE attachment_path <> '';
`)
	return err
}

func migrateV11(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS recurrence_series (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  item_id TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  rule_version INTEGER NOT NULL DEFAULT 1,
  rule_json TEXT NOT NULL,
  timezone TEXT NOT NULL,
  starts_at TEXT NOT NULL,
  ends_at TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
  FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_recurrence_series_workspace_status ON recurrence_series(workspace_id, status);
CREATE TABLE IF NOT EXISTS recurrence_occurrences (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  series_id TEXT NOT NULL,
  occurrence_key TEXT NOT NULL,
  scheduled_start TEXT NOT NULL,
  scheduled_end TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'scheduled',
  override_json TEXT NOT NULL DEFAULT '',
  completed_at TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(series_id, occurrence_key),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
  FOREIGN KEY(series_id) REFERENCES recurrence_series(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_recurrence_occurrences_range ON recurrence_occurrences(workspace_id, scheduled_start);
CREATE TABLE IF NOT EXISTS recurrence_exceptions (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  series_id TEXT NOT NULL,
  occurrence_key TEXT NOT NULL,
  action TEXT NOT NULL,
  override_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  UNIQUE(series_id, occurrence_key),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
  FOREIGN KEY(series_id) REFERENCES recurrence_series(id) ON DELETE CASCADE
);
`)
	return err
}

func migrateV4(tx *sql.Tx) error {
	return ensureColumnTx(tx, "projects", "version", "INTEGER NOT NULL DEFAULT 1")
}

func migrateV5(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE assistant_plans (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  state TEXT NOT NULL,
  plan_json TEXT NOT NULL,
  requires_confirmation INTEGER NOT NULL DEFAULT 0,
  confirmation_expires_at TEXT NOT NULL DEFAULT '',
  actor TEXT NOT NULL DEFAULT 'assistant',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  started_at TEXT NOT NULL DEFAULT '',
  completed_at TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_assistant_plans_workspace_idempotency
  ON assistant_plans(workspace_id, idempotency_key);
CREATE INDEX idx_assistant_plans_workspace_updated
  ON assistant_plans(workspace_id, updated_at DESC);
CREATE TABLE assistant_plan_actions (
  plan_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  action_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  operation TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending',
  error TEXT NOT NULL DEFAULT '',
  before_json TEXT NOT NULL DEFAULT '',
  after_json TEXT NOT NULL DEFAULT '',
  result_json TEXT NOT NULL DEFAULT '',
  event_ids_json TEXT NOT NULL DEFAULT '[]',
  actor TEXT NOT NULL DEFAULT 'assistant',
  conversation_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  completed_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(plan_id, action_id),
  FOREIGN KEY(plan_id) REFERENCES assistant_plans(id) ON DELETE CASCADE,
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);
CREATE INDEX idx_assistant_plan_actions_workspace ON assistant_plan_actions(workspace_id, plan_id, ordinal);
`)
	return err
}

func migrateV6(tx *sql.Tx) error {
	if err := ensureColumnTx(tx, "projects", "archived_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnTx(tx, "projects", "archived_from_status", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnTx(tx, "assistant_plans", "lease_until", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnTx(tx, "assistant_plans", "lease_token", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS project_events (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_project_events_project_created ON project_events(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_events_workspace_created ON project_events(workspace_id, created_at DESC);
`)
	return err
}

func migrateV7(tx *sql.Tx) error {
	if err := ensureColumnTx(tx, "assistant_plan_actions", "lease_token", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnTx(tx, "assistant_plan_actions", "lease_until", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

func migrateV8(tx *sql.Tx) error {
	return ensureColumnTx(tx, "assistant_plans", "source_request_hash", "TEXT NOT NULL DEFAULT ''")
}

func migrateV9(tx *sql.Tx) error {
	if err := ensureColumnTx(tx, "assistant_conversations", "title", "TEXT NOT NULL DEFAULT '新对话'"); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE assistant_conversations
SET title=COALESCE(NULLIF(SUBSTR((SELECT content FROM assistant_turns
  WHERE assistant_turns.workspace_id=assistant_conversations.workspace_id
    AND assistant_turns.conversation_id=assistant_conversations.id
    AND role='user' ORDER BY rowid LIMIT 1), 1, 40), ''), '新对话')
WHERE title='' OR title='新对话'`)
	return err
}

func migrateV3(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE assistant_conversations (
  id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(workspace_id, id),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);
CREATE INDEX idx_assistant_conversations_workspace_updated ON assistant_conversations(workspace_id, updated_at DESC);
CREATE TABLE assistant_turns (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  structured_result TEXT NOT NULL DEFAULT '',
  structured_result_ref TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  FOREIGN KEY(workspace_id, conversation_id) REFERENCES assistant_conversations(workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_assistant_turns_workspace_conversation_created ON assistant_turns(workspace_id, conversation_id, created_at);
`)
	return err
}

func migrateV2(tx *sql.Tx) error {
	return ensureColumnTx(tx, "items", "archived_from_status", "TEXT NOT NULL DEFAULT ''")
}

func (s *Store) defaultWorkspaceID() string { return defaultPersonalWorkspaceID }

func migrateV1(tx *sql.Tx) error {
	const schema = `
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS device_tokens (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  last_used_at TEXT NOT NULL,
  revoked_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS captures (
  id TEXT PRIMARY KEY,
  source_type TEXT NOT NULL,
  raw_text TEXT NOT NULL DEFAULT '',
  attachment_name TEXT NOT NULL DEFAULT '',
  attachment_mime TEXT NOT NULL DEFAULT '',
  attachment_path TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  ai_result TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_captures_status_created ON captures(status, created_at DESC);
CREATE TABLE IF NOT EXISTS items (
  id TEXT PRIMARY KEY,
  capture_id TEXT,
  type TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  start_at TEXT NOT NULL DEFAULT '',
  end_at TEXT NOT NULL DEFAULT '',
  due_at TEXT NOT NULL DEFAULT '',
  reminder_at TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL,
  all_day INTEGER NOT NULL DEFAULT 0,
  recurrence_rule TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  project TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  location TEXT NOT NULL DEFAULT '',
  people_json TEXT NOT NULL DEFAULT '[]',
  version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(capture_id) REFERENCES captures(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_items_status_due ON items(status, due_at);
CREATE INDEX IF NOT EXISTS idx_items_status_start ON items(status, start_at);
CREATE TABLE IF NOT EXISTS reminders (
  id TEXT PRIMARY KEY,
  item_id TEXT NOT NULL,
  remind_at TEXT NOT NULL,
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  sent_at TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_reminders_due ON reminders(status, remind_at);
CREATE TABLE IF NOT EXISTS push_subscriptions (
  endpoint TEXT PRIMARY KEY,
  p256dh TEXT NOT NULL,
  auth TEXT NOT NULL,
  user_agent TEXT NOT NULL DEFAULT '',
  active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  priority INTEGER NOT NULL DEFAULT 0,
  color TEXT NOT NULL DEFAULT '',
  goal TEXT NOT NULL DEFAULT '',
  deadline TEXT NOT NULL DEFAULT '',
  review_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_projects_status_updated ON projects(status, updated_at DESC);
CREATE TABLE IF NOT EXISTS item_events (
  id TEXT PRIMARY KEY,
  item_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_item_events_item_created ON item_events(item_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_item_events_created ON item_events(created_at DESC);
CREATE TABLE IF NOT EXISTS memories (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  memory_key TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT 'global',
  status TEXT NOT NULL DEFAULT 'proposed',
  evidence_count INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_key_status ON memories(memory_key, status) WHERE memory_key<>'';
CREATE TABLE IF NOT EXISTS correction_events (
  id TEXT PRIMARY KEY,
  capture_id TEXT NOT NULL,
  field TEXT NOT NULL,
  context TEXT NOT NULL DEFAULT '',
  before_json TEXT NOT NULL,
  after_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(capture_id) REFERENCES captures(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_corrections_field_context ON correction_events(field, context, created_at DESC);
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'personal',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(owner_user_id) REFERENCES users(id)
);
CREATE TABLE IF NOT EXISTS workspace_members (
  workspace_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'owner',
  created_at TEXT NOT NULL,
  PRIMARY KEY(workspace_id, user_id),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);`
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	columns := []struct{ name, definition string }{
		{"certainty", "TEXT NOT NULL DEFAULT 'confirmed'"},
		{"duration_minutes", "INTEGER NOT NULL DEFAULT 0"},
		{"available_from", "TEXT NOT NULL DEFAULT ''"},
		{"available_until", "TEXT NOT NULL DEFAULT ''"},
		{"project_id", "TEXT NOT NULL DEFAULT ''"},
		{"completed_at", "TEXT NOT NULL DEFAULT ''"},
		{"cancelled_at", "TEXT NOT NULL DEFAULT ''"},
		{"archived_at", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := ensureColumnTx(tx, "items", column.name, column.definition); err != nil {
			return err
		}
	}
	for _, table := range []string{"captures", "items", "projects", "memories", "correction_events", "push_subscriptions", "device_tokens"} {
		if err := ensureColumnTx(tx, table, "workspace_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	now := nowString()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO users(id, username, display_name, created_at, updated_at) VALUES(?, ?, ?, ?, ?)`, defaultPersonalUserID, "personal", "Personal", now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO workspaces(id, owner_user_id, name, kind, created_at, updated_at) VALUES(?, ?, ?, 'personal', ?, ?)`, defaultPersonalWorkspaceID, defaultPersonalUserID, "Personal", now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO workspace_members(workspace_id, user_id, role, created_at) VALUES(?, ?, 'owner', ?)`, defaultPersonalWorkspaceID, defaultPersonalUserID, now); err != nil {
		return err
	}
	for _, table := range []string{"captures", "items", "projects", "memories", "correction_events", "push_subscriptions", "device_tokens"} {
		if _, err := tx.Exec("UPDATE "+table+" SET workspace_id=? WHERE workspace_id='' OR workspace_id IS NULL", defaultPersonalWorkspaceID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("UPDATE items SET completed_at=updated_at WHERE status='done' AND completed_at=''"); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE items SET cancelled_at=updated_at WHERE status='cancelled' AND cancelled_at=''"); err != nil {
		return err
	}
	return migrateProjectsTx(tx)
}

func ensureColumnTx(tx *sql.Tx, table, name, definition string) error {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec("ALTER TABLE " + table + " ADD COLUMN " + name + " " + definition)
	return err
}

func migrateProjectsTx(tx *sql.Tx) error {
	rows, err := tx.Query("SELECT DISTINCT TRIM(project) FROM items WHERE TRIM(project)<>''")
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, name := range names {
		now := nowString()
		if _, err := tx.Exec("INSERT OR IGNORE INTO projects(id, name, created_at, updated_at, workspace_id) VALUES(?, ?, ?, ?, ?)", newID("project"), name, now, now, defaultPersonalWorkspaceID); err != nil {
			return err
		}
	}
	_, err = tx.Exec("UPDATE items SET project_id=COALESCE((SELECT id FROM projects WHERE name=items.project COLLATE NOCASE AND workspace_id=items.workspace_id), '') WHERE project_id='' AND TRIM(project)<>''")
	return err
}
