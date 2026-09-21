package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var errConflict = errors.New("version conflict")

type Store struct {
	db *sql.DB
}

func openStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
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
CREATE INDEX IF NOT EXISTS idx_item_events_created ON item_events(created_at DESC);`
	const memorySchema = `
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
CREATE INDEX IF NOT EXISTS idx_corrections_field_context ON correction_events(field, context, created_at DESC);`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	if _, err := s.db.Exec(memorySchema); err != nil {
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
		if err := s.ensureColumn("items", column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec("UPDATE items SET completed_at=updated_at WHERE status='done' AND completed_at=''"); err != nil {
		return err
	}
	if _, err := s.db.Exec("UPDATE items SET cancelled_at=updated_at WHERE status='cancelled' AND cancelled_at=''"); err != nil {
		return err
	}
	return s.migrateProjects()
}

func (s *Store) ensureColumn(table, name, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
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
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + name + " " + definition)
	return err
}

func (s *Store) migrateProjects() error {
	rows, err := s.db.Query("SELECT DISTINCT TRIM(project) FROM items WHERE TRIM(project)<>''")
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
		if _, err := s.db.Exec("INSERT OR IGNORE INTO projects(id, name, created_at, updated_at) VALUES(?, ?, ?, ?)", newID("project"), name, now, now); err != nil {
			return err
		}
	}
	_, err = s.db.Exec("UPDATE items SET project_id=COALESCE((SELECT id FROM projects WHERE name=items.project COLLATE NOCASE), '') WHERE project_id='' AND TRIM(project)<>''")
	return err
}

func newID(prefix string) string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf)
}

func newSecret(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Store) adminPasswordHash(fallback string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = 'admin_password_hash'").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	return value, err
}

func (s *Store) setAdminPasswordHash(value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value, updated_at) VALUES('admin_password_hash', ?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, value, nowString())
	return err
}

func (s *Store) createSession(token string, expires time.Time) error {
	now := nowString()
	_, err := s.db.Exec("INSERT INTO sessions(token_hash, expires_at, created_at) VALUES(?, ?, ?)", hashSecret(token), expires.UTC().Format(time.RFC3339Nano), now)
	return err
}

func (s *Store) validSession(token string) bool {
	if token == "" {
		return false
	}
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE token_hash=? AND expires_at>?", hashSecret(token), nowString()).Scan(&count)
	return err == nil && count == 1
}

func (s *Store) deleteSession(token string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash=?", hashSecret(token))
	return err
}

func (s *Store) revokeAllSessions() error {
	_, err := s.db.Exec("DELETE FROM sessions")
	return err
}

func (s *Store) createDeviceToken(name string) (string, string, error) {
	secret, err := newSecret(32)
	if err != nil {
		return "", "", err
	}
	id := newID("device")
	now := nowString()
	_, err = s.db.Exec("INSERT INTO device_tokens(id, name, token_hash, created_at, last_used_at) VALUES(?, ?, ?, ?, ?)", id, strings.TrimSpace(name), hashSecret(secret), now, now)
	return id, secret, err
}

func (s *Store) validDeviceToken(token string) bool {
	if token == "" {
		return false
	}
	now := nowString()
	result, err := s.db.Exec("UPDATE device_tokens SET last_used_at=? WHERE token_hash=? AND revoked_at=''", now, hashSecret(token))
	if err != nil {
		return false
	}
	count, _ := result.RowsAffected()
	return count == 1
}

func (s *Store) createCapture(capture Capture) (Capture, error) {
	if capture.ID == "" {
		capture.ID = newID("capture")
	}
	now := nowString()
	capture.CreatedAt, capture.UpdatedAt = now, now
	if capture.Status == "" {
		capture.Status = "received"
	}
	_, err := s.db.Exec(`INSERT INTO captures(id, source_type, raw_text, attachment_name, attachment_mime, attachment_path, status, ai_result, error, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, '', '', ?, ?)`, capture.ID, capture.SourceType, capture.RawText, capture.AttachmentName, capture.AttachmentMime, capture.AttachmentPath, capture.Status, now, now)
	capture.HasAttachment = capture.AttachmentPath != ""
	return capture, err
}

func scanCapture(scanner interface{ Scan(...any) error }) (Capture, error) {
	var c Capture
	var ai string
	err := scanner.Scan(&c.ID, &c.SourceType, &c.RawText, &c.AttachmentName, &c.AttachmentMime, &c.AttachmentPath, &c.Status, &ai, &c.Error, &c.CreatedAt, &c.UpdatedAt)
	if ai != "" {
		c.AIResult = json.RawMessage(ai)
	}
	c.HasAttachment = c.AttachmentPath != ""
	return c, err
}

const captureColumns = "id, source_type, raw_text, attachment_name, attachment_mime, attachment_path, status, ai_result, error, created_at, updated_at"

func (s *Store) getCapture(id string) (Capture, error) {
	return scanCapture(s.db.QueryRow("SELECT "+captureColumns+" FROM captures WHERE id=?", id))
}

func (s *Store) listCaptures(limit int) ([]Capture, error) {
	rows, err := s.db.Query("SELECT "+captureColumns+" FROM captures ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]Capture, 0)
	for rows.Next() {
		value, err := scanCapture(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) deleteCapture(id string) error {
	result, err := s.db.Exec("DELETE FROM captures WHERE id=?", id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) setCaptureParsing(id string) error {
	_, err := s.db.Exec("UPDATE captures SET status='parsing', error='', updated_at=? WHERE id=?", nowString(), id)
	return err
}

func (s *Store) setCaptureResult(id string, result ParseResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE captures SET status='needs_review', ai_result=?, error='', updated_at=? WHERE id=?", string(data), nowString(), id)
	return err
}

func (s *Store) setCaptureError(id string, parseErr error) error {
	_, err := s.db.Exec("UPDATE captures SET status='failed', error=?, updated_at=? WHERE id=?", parseErr.Error(), nowString(), id)
	return err
}

func normalizeCandidate(candidate Candidate, defaultTimezone string) Candidate {
	candidate.Title = strings.TrimSpace(candidate.Title)
	candidate.Type = strings.ToLower(strings.TrimSpace(candidate.Type))
	if candidate.Type != "event" && candidate.Type != "reminder" && candidate.Type != "note" {
		candidate.Type = "task"
	}
	candidate.Certainty = strings.ToLower(strings.TrimSpace(candidate.Certainty))
	if candidate.Certainty != "tentative" {
		candidate.Certainty = "confirmed"
	}
	if candidate.DurationMinutes < 0 {
		candidate.DurationMinutes = 0
	}
	if candidate.DurationMinutes > 43200 {
		candidate.DurationMinutes = 43200
	}
	if candidate.Timezone == "" {
		candidate.Timezone = defaultTimezone
	}
	if candidate.Priority < 0 {
		candidate.Priority = 0
	}
	if candidate.Priority > 3 {
		candidate.Priority = 3
	}
	candidate.Project = strings.TrimSpace(candidate.Project)
	if candidate.Tags == nil {
		candidate.Tags = []string{}
	}
	if candidate.People == nil {
		candidate.People = []string{}
	}
	candidate.StartAt = canonicalTime(candidate.StartAt)
	candidate.EndAt = canonicalTime(candidate.EndAt)
	candidate.DueAt = canonicalTime(candidate.DueAt)
	candidate.ReminderAt = canonicalTime(candidate.ReminderAt)
	candidate.AvailableFrom = canonicalTime(candidate.AvailableFrom)
	candidate.AvailableUntil = canonicalTime(candidate.AvailableUntil)
	return candidate
}

func canonicalTime(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format(time.RFC3339)
}

func sourceForCapture(captureID string) string {
	if captureID == "" {
		return "manual"
	}
	return "capture"
}

func ensureProjectTx(tx *sql.Tx, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	var id string
	err := tx.QueryRow("SELECT id FROM projects WHERE name=? COLLATE NOCASE", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id, now := newID("project"), nowString()
	if _, err := tx.Exec("INSERT INTO projects(id, name, created_at, updated_at) VALUES(?, ?, ?, ?)", id, name, now, now); err != nil {
		if queryErr := tx.QueryRow("SELECT id FROM projects WHERE name=? COLLATE NOCASE", name).Scan(&id); queryErr == nil {
			return id, nil
		}
		return "", err
	}
	return id, nil
}

func recordItemEventTx(tx *sql.Tx, itemID, eventType string, data any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO item_events(id, item_id, event_type, data_json, created_at) VALUES(?, ?, ?, ?, ?)", newID("event"), itemID, eventType, string(encoded), nowString())
	return err
}

func (s *Store) createItems(captureID string, candidates []Candidate, defaultTimezone string) ([]Item, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	created := make([]Item, 0, len(candidates))
	for _, raw := range candidates {
		candidate := normalizeCandidate(raw, defaultTimezone)
		if candidate.Title == "" {
			return nil, errors.New("item title is required")
		}
		tags, _ := json.Marshal(candidate.Tags)
		people, _ := json.Marshal(candidate.People)
		now, id := nowString(), newID("item")
		projectID, err := ensureProjectTx(tx, candidate.Project)
		if err != nil {
			return nil, err
		}
		var captureReference any
		if captureID != "" {
			captureReference = captureID
		}
		_, err = tx.Exec(`INSERT INTO items(id, capture_id, type, title, description, start_at, end_at, due_at, reminder_at, timezone, all_day, recurrence_rule, priority, status, certainty, duration_minutes, available_from, available_until, project_id, project, tags_json, location, people_json, version, completed_at, cancelled_at, archived_at, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'todo', ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, '', '', '', ?, ?)`, id, captureReference, candidate.Type, candidate.Title, candidate.Description, candidate.StartAt, candidate.EndAt, candidate.DueAt, candidate.ReminderAt, candidate.Timezone, candidate.AllDay, candidate.RecurrenceRule, candidate.Priority, candidate.Certainty, candidate.DurationMinutes, candidate.AvailableFrom, candidate.AvailableUntil, projectID, candidate.Project, string(tags), candidate.Location, string(people), now, now)
		if err != nil {
			return nil, err
		}
		if candidate.ReminderAt != "" {
			_, err = tx.Exec("INSERT INTO reminders(id, item_id, remind_at, status, created_at, updated_at) VALUES(?, ?, ?, 'pending', ?, ?)", newID("reminder"), id, candidate.ReminderAt, now, now)
			if err != nil {
				return nil, err
			}
		}
		if err := recordItemEventTx(tx, id, "created", map[string]any{"source": sourceForCapture(captureID), "captureId": captureID}); err != nil {
			return nil, err
		}
		created = append(created, Item{ID: id, CaptureID: captureID, Type: candidate.Type, Title: candidate.Title, Description: candidate.Description, StartAt: candidate.StartAt, EndAt: candidate.EndAt, DueAt: candidate.DueAt, ReminderAt: candidate.ReminderAt, Timezone: candidate.Timezone, AllDay: candidate.AllDay, RecurrenceRule: candidate.RecurrenceRule, Priority: candidate.Priority, Status: "todo", Certainty: candidate.Certainty, DurationMinutes: candidate.DurationMinutes, AvailableFrom: candidate.AvailableFrom, AvailableUntil: candidate.AvailableUntil, ProjectID: projectID, Project: candidate.Project, Tags: candidate.Tags, Location: candidate.Location, People: candidate.People, Version: 1, CreatedAt: now, UpdatedAt: now})
	}
	if captureID != "" {
		if _, err := tx.Exec("UPDATE captures SET status='confirmed', updated_at=? WHERE id=?", nowString(), captureID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

const itemColumns = "id, COALESCE(capture_id, ''), type, title, description, start_at, end_at, due_at, reminder_at, timezone, all_day, recurrence_rule, priority, status, certainty, duration_minutes, available_from, available_until, project_id, project, tags_json, location, people_json, version, completed_at, cancelled_at, archived_at, created_at, updated_at"

func scanItem(scanner interface{ Scan(...any) error }) (Item, error) {
	var item Item
	var allDay int
	var tags, people string
	err := scanner.Scan(&item.ID, &item.CaptureID, &item.Type, &item.Title, &item.Description, &item.StartAt, &item.EndAt, &item.DueAt, &item.ReminderAt, &item.Timezone, &allDay, &item.RecurrenceRule, &item.Priority, &item.Status, &item.Certainty, &item.DurationMinutes, &item.AvailableFrom, &item.AvailableUntil, &item.ProjectID, &item.Project, &tags, &item.Location, &people, &item.Version, &item.CompletedAt, &item.CancelledAt, &item.ArchivedAt, &item.CreatedAt, &item.UpdatedAt)
	item.AllDay = allDay != 0
	_ = json.Unmarshal([]byte(tags), &item.Tags)
	_ = json.Unmarshal([]byte(people), &item.People)
	if item.Tags == nil {
		item.Tags = []string{}
	}
	if item.People == nil {
		item.People = []string{}
	}
	return item, err
}

func (s *Store) getItem(id string) (Item, error) {
	return scanItem(s.db.QueryRow("SELECT "+itemColumns+" FROM items WHERE id=?", id))
}

func (s *Store) listItems(status string, limit int) ([]Item, error) {
	query := "SELECT " + itemColumns + " FROM items"
	args := []any{}
	if status != "" {
		query += " WHERE status=?"
		args = append(args, status)
	}
	query += " ORDER BY CASE WHEN start_at<>'' THEN start_at WHEN due_at<>'' THEN due_at ELSE created_at END ASC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) listProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT p.id, p.name, p.description, p.status, p.priority, p.color, p.goal, p.deadline, p.review_at,
  COUNT(i.id),
  SUM(CASE WHEN i.status IN ('inbox','todo','doing') THEN 1 ELSE 0 END),
  SUM(CASE WHEN i.status='done' THEN 1 ELSE 0 END),
  p.created_at, p.updated_at
FROM projects p
LEFT JOIN items i ON i.project_id=p.id
GROUP BY p.id
ORDER BY CASE p.status WHEN 'active' THEN 0 WHEN 'waiting' THEN 1 WHEN 'blocked' THEN 2 WHEN 'completed' THEN 3 ELSE 4 END, p.priority DESC, p.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]Project, 0)
	for rows.Next() {
		var value Project
		if err := rows.Scan(&value.ID, &value.Name, &value.Description, &value.Status, &value.Priority, &value.Color, &value.Goal, &value.Deadline, &value.ReviewAt, &value.ItemCount, &value.OpenCount, &value.DoneCount, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) getProject(id string) (Project, error) {
	var value Project
	err := s.db.QueryRow(`SELECT p.id, p.name, p.description, p.status, p.priority, p.color, p.goal, p.deadline, p.review_at,
  COUNT(i.id), SUM(CASE WHEN i.status IN ('inbox','todo','doing') THEN 1 ELSE 0 END), SUM(CASE WHEN i.status='done' THEN 1 ELSE 0 END),
  p.created_at, p.updated_at
FROM projects p LEFT JOIN items i ON i.project_id=p.id WHERE p.id=? GROUP BY p.id`, id).Scan(&value.ID, &value.Name, &value.Description, &value.Status, &value.Priority, &value.Color, &value.Goal, &value.Deadline, &value.ReviewAt, &value.ItemCount, &value.OpenCount, &value.DoneCount, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func normalizeProject(project Project) Project {
	project.Name = strings.TrimSpace(project.Name)
	project.Description = strings.TrimSpace(project.Description)
	project.Goal = strings.TrimSpace(project.Goal)
	project.Status = strings.ToLower(strings.TrimSpace(project.Status))
	switch project.Status {
	case "active", "waiting", "blocked", "completed", "archived":
	default:
		project.Status = "active"
	}
	if project.Priority < 0 {
		project.Priority = 0
	}
	if project.Priority > 3 {
		project.Priority = 3
	}
	project.Deadline = canonicalTime(project.Deadline)
	project.ReviewAt = canonicalTime(project.ReviewAt)
	return project
}

func (s *Store) createProject(project Project) (Project, error) {
	project = normalizeProject(project)
	if project.Name == "" {
		return Project{}, errors.New("project name is required")
	}
	project.ID, project.CreatedAt, project.UpdatedAt = newID("project"), nowString(), nowString()
	_, err := s.db.Exec(`INSERT INTO projects(id, name, description, status, priority, color, goal, deadline, review_at, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, project.ID, project.Name, project.Description, project.Status, project.Priority, project.Color, project.Goal, project.Deadline, project.ReviewAt, project.CreatedAt, project.UpdatedAt)
	if err != nil {
		return Project{}, err
	}
	return s.getProject(project.ID)
}

func (s *Store) updateProject(project Project) (Project, error) {
	project = normalizeProject(project)
	if project.Name == "" {
		return Project{}, errors.New("project name is required")
	}
	before, err := s.getProject(project.ID)
	if err != nil {
		return Project{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	now := nowString()
	if _, err := tx.Exec(`UPDATE projects SET name=?, description=?, status=?, priority=?, color=?, goal=?, deadline=?, review_at=?, updated_at=? WHERE id=?`, project.Name, project.Description, project.Status, project.Priority, project.Color, project.Goal, project.Deadline, project.ReviewAt, now, project.ID); err != nil {
		return Project{}, err
	}
	if project.Name != before.Name {
		if _, err := tx.Exec("UPDATE items SET project=?, updated_at=? WHERE project_id=?", project.Name, now, project.ID); err != nil {
			return Project{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	return s.getProject(project.ID)
}

func (s *Store) listItemEvents(itemID string, limit int) ([]ItemEvent, error) {
	query := "SELECT id, item_id, event_type, data_json, created_at FROM item_events"
	args := []any{}
	if itemID != "" {
		query += " WHERE item_id=?"
		args = append(args, itemID)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ItemEvent, 0)
	for rows.Next() {
		var value ItemEvent
		var data string
		if err := rows.Scan(&value.ID, &value.ItemID, &value.EventType, &data, &value.CreatedAt); err != nil {
			return nil, err
		}
		value.Data = json.RawMessage(data)
		values = append(values, value)
	}
	return values, rows.Err()
}

func normalizeMemory(memory Memory) Memory {
	memory.Kind = strings.ToLower(strings.TrimSpace(memory.Kind))
	switch memory.Kind {
	case "preference", "procedure", "example":
	default:
		memory.Kind = "preference"
	}
	memory.Content = strings.TrimSpace(memory.Content)
	memory.Scope = strings.TrimSpace(memory.Scope)
	if memory.Scope == "" {
		memory.Scope = "global"
	}
	memory.Status = strings.ToLower(strings.TrimSpace(memory.Status))
	switch memory.Status {
	case "proposed", "active", "dismissed", "forgotten":
	default:
		memory.Status = "active"
	}
	if memory.EvidenceCount < 1 {
		memory.EvidenceCount = 1
	}
	return memory
}

func (s *Store) listMemories(status string) ([]Memory, error) {
	query := "SELECT id, kind, memory_key, content, scope, status, evidence_count, created_at, updated_at FROM memories"
	args := []any{}
	if status != "" {
		query += " WHERE status=?"
		args = append(args, status)
	}
	query += " ORDER BY CASE status WHEN 'proposed' THEN 0 WHEN 'active' THEN 1 ELSE 2 END, updated_at DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]Memory, 0)
	for rows.Next() {
		var value Memory
		if err := rows.Scan(&value.ID, &value.Kind, &value.Key, &value.Content, &value.Scope, &value.Status, &value.EvidenceCount, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) createMemory(memory Memory) (Memory, error) {
	memory = normalizeMemory(memory)
	if memory.Content == "" {
		return Memory{}, errors.New("memory content is required")
	}
	memory.ID, memory.CreatedAt, memory.UpdatedAt = newID("memory"), nowString(), nowString()
	_, err := s.db.Exec(`INSERT INTO memories(id, kind, memory_key, content, scope, status, evidence_count, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, memory.ID, memory.Kind, memory.Key, memory.Content, memory.Scope, memory.Status, memory.EvidenceCount, memory.CreatedAt, memory.UpdatedAt)
	return memory, err
}

func (s *Store) updateMemory(memory Memory) (Memory, error) {
	memory = normalizeMemory(memory)
	if memory.Content == "" {
		return Memory{}, errors.New("memory content is required")
	}
	memory.UpdatedAt = nowString()
	result, err := s.db.Exec(`UPDATE memories SET kind=?, content=?, scope=?, status=?, evidence_count=?, updated_at=? WHERE id=?`, memory.Kind, memory.Content, memory.Scope, memory.Status, memory.EvidenceCount, memory.UpdatedAt, memory.ID)
	if err != nil {
		return Memory{}, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return Memory{}, sql.ErrNoRows
	}
	err = s.db.QueryRow("SELECT memory_key, created_at FROM memories WHERE id=?", memory.ID).Scan(&memory.Key, &memory.CreatedAt)
	return memory, err
}

func (s *Store) activeMemoryPrompt() (string, error) {
	memories, err := s.listMemories("active")
	if err != nil {
		return "", err
	}
	if len(memories) == 0 {
		return "", nil
	}
	var lines []string
	for index, memory := range memories {
		if index >= 20 {
			break
		}
		lines = append(lines, "- ["+memory.ID+"] "+memory.Content)
	}
	return strings.Join(lines, "\n"), nil
}

func fuzzyPeriod(text string) string {
	for _, period := range []string{"早上", "上午", "中午", "下午", "傍晚", "晚上", "下班后"} {
		if strings.Contains(text, period) {
			return period
		}
	}
	return ""
}

func clockPart(value string, location *time.Location) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return parsed.In(location).Format("15:04")
}

func (s *Store) recordCaptureCorrections(capture Capture, confirmed ParseResult, timezone string) error {
	var original ParseResult
	if len(capture.AIResult) == 0 || json.Unmarshal(capture.AIResult, &original) != nil {
		return nil
	}
	location, _ := time.LoadLocation(timezone)
	if location == nil {
		location = time.UTC
	}
	period := fuzzyPeriod(capture.RawText)
	fields := []struct {
		name string
		get  func(Candidate) string
	}{
		{"type", func(v Candidate) string { return v.Type }},
		{"title", func(v Candidate) string { return v.Title }},
		{"startAt", func(v Candidate) string { return v.StartAt }},
		{"dueAt", func(v Candidate) string { return v.DueAt }},
		{"reminderAt", func(v Candidate) string { return v.ReminderAt }},
		{"project", func(v Candidate) string { return v.Project }},
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, after := range confirmed.Items {
		if index >= len(original.Items) {
			break
		}
		before := original.Items[index]
		for _, field := range fields {
			oldValue, newValue := field.get(before), field.get(after)
			if oldValue == newValue {
				continue
			}
			oldJSON, _ := json.Marshal(oldValue)
			newJSON, _ := json.Marshal(newValue)
			if _, err := tx.Exec(`INSERT INTO correction_events(id, capture_id, field, context, before_json, after_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, newID("correction"), capture.ID, field.name, period, string(oldJSON), string(newJSON), nowString()); err != nil {
				return err
			}
			if period != "" && (field.name == "startAt" || field.name == "dueAt" || field.name == "reminderAt") {
				clock := clockPart(newValue, location)
				if clock == "" {
					continue
				}
				var count int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM correction_events WHERE field=? AND context=? AND after_json=?`, field.name, period, string(newJSON)).Scan(&count); err != nil {
					return err
				}
				if count >= 2 {
					key := "fuzzy-time:" + period + ":" + field.name
					content := "当用户说“" + period + "”时，默认将" + field.name + "设为当地时间 " + clock + "。"
					_, err := tx.Exec(`INSERT INTO memories(id, kind, memory_key, content, scope, status, evidence_count, created_at, updated_at) VALUES(?, 'preference', ?, ?, 'global', 'proposed', ?, ?, ?) ON CONFLICT(memory_key, status) WHERE memory_key<>'' DO UPDATE SET content=excluded.content, evidence_count=excluded.evidence_count, updated_at=excluded.updated_at`, newID("memory"), key, content, count, nowString(), nowString())
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return tx.Commit()
}

func (s *Store) updateItem(item Item) (Item, error) {
	before, err := s.getItem(item.ID)
	if err != nil {
		return Item{}, err
	}
	candidate := normalizeCandidate(Candidate{
		Type: item.Type, Title: item.Title, Description: item.Description, StartAt: item.StartAt,
		EndAt: item.EndAt, DueAt: item.DueAt, ReminderAt: item.ReminderAt, Timezone: item.Timezone,
		AllDay: item.AllDay, RecurrenceRule: item.RecurrenceRule, Priority: item.Priority,
		Certainty: item.Certainty, DurationMinutes: item.DurationMinutes, AvailableFrom: item.AvailableFrom,
		AvailableUntil: item.AvailableUntil, Project: item.Project, Tags: item.Tags, Location: item.Location, People: item.People,
	}, item.Timezone)
	if candidate.Title == "" {
		return Item{}, errors.New("title is required")
	}
	item.Type, item.Title, item.Description = candidate.Type, candidate.Title, candidate.Description
	item.StartAt, item.EndAt, item.DueAt, item.ReminderAt = candidate.StartAt, candidate.EndAt, candidate.DueAt, candidate.ReminderAt
	item.Timezone, item.AllDay, item.RecurrenceRule, item.Priority = candidate.Timezone, candidate.AllDay, candidate.RecurrenceRule, candidate.Priority
	item.Certainty, item.DurationMinutes = candidate.Certainty, candidate.DurationMinutes
	item.AvailableFrom, item.AvailableUntil = candidate.AvailableFrom, candidate.AvailableUntil
	item.Project, item.Tags, item.Location, item.People = candidate.Project, candidate.Tags, candidate.Location, candidate.People
	tags, _ := json.Marshal(candidate.Tags)
	people, _ := json.Marshal(candidate.People)
	now := nowString()
	item.CompletedAt, item.CancelledAt, item.ArchivedAt = before.CompletedAt, before.CancelledAt, before.ArchivedAt
	if item.Status != before.Status {
		switch item.Status {
		case "done":
			item.CompletedAt, item.CancelledAt, item.ArchivedAt = now, "", ""
		case "cancelled":
			item.CompletedAt, item.CancelledAt, item.ArchivedAt = "", now, ""
		case "archived":
			item.ArchivedAt = now
		default:
			item.CompletedAt, item.CancelledAt, item.ArchivedAt = "", "", ""
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	projectID, err := ensureProjectTx(tx, item.Project)
	if err != nil {
		return Item{}, err
	}
	item.ProjectID = projectID
	result, err := tx.Exec(`UPDATE items SET type=?, title=?, description=?, start_at=?, end_at=?, due_at=?, reminder_at=?, timezone=?, all_day=?, recurrence_rule=?, priority=?, status=?, certainty=?, duration_minutes=?, available_from=?, available_until=?, project_id=?, project=?, tags_json=?, location=?, people_json=?, completed_at=?, cancelled_at=?, archived_at=?, version=version+1, updated_at=? WHERE id=? AND version=?`, item.Type, item.Title, item.Description, item.StartAt, item.EndAt, item.DueAt, item.ReminderAt, item.Timezone, item.AllDay, item.RecurrenceRule, item.Priority, item.Status, item.Certainty, item.DurationMinutes, item.AvailableFrom, item.AvailableUntil, projectID, item.Project, string(tags), item.Location, string(people), item.CompletedAt, item.CancelledAt, item.ArchivedAt, now, item.ID, item.Version)
	if err != nil {
		return Item{}, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return Item{}, errConflict
	}
	if _, err := tx.Exec("DELETE FROM reminders WHERE item_id=? AND status='pending'", item.ID); err != nil {
		return Item{}, err
	}
	if item.ReminderAt != "" && item.Status != "done" && item.Status != "cancelled" && item.Status != "archived" {
		_, err = tx.Exec("INSERT INTO reminders(id, item_id, remind_at, status, created_at, updated_at) VALUES(?, ?, ?, 'pending', ?, ?)", newID("reminder"), item.ID, item.ReminderAt, now, now)
		if err != nil {
			return Item{}, err
		}
	}
	eventType := "updated"
	if item.Status != before.Status {
		switch item.Status {
		case "done":
			eventType = "completed"
		case "cancelled":
			eventType = "cancelled"
		case "archived":
			eventType = "archived"
		case "todo", "doing":
			eventType = "reopened"
		default:
			eventType = "status_changed"
		}
	}
	item.Version = before.Version + 1
	item.UpdatedAt = now
	if err := recordItemEventTx(tx, item.ID, eventType, map[string]any{"before": before, "after": item}); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return s.getItem(item.ID)
}

func (s *Store) completeItem(id string) (Item, error) {
	return s.setItemStatus(id, "done")
}

func (s *Store) cancelItem(id string) (Item, error) { return s.setItemStatus(id, "cancelled") }

func (s *Store) reopenItem(id string) (Item, error) { return s.setItemStatus(id, "todo") }

func (s *Store) archiveItem(id string) (Item, error) { return s.setItemStatus(id, "archived") }

func (s *Store) setItemStatus(id, status string) (Item, error) {
	item, err := s.getItem(id)
	if err != nil {
		return Item{}, err
	}
	item.Status = status
	return s.updateItem(item)
}

func (s *Store) snoozeItem(id string, remindAt string) (Item, error) {
	before, err := s.getItem(id)
	if err != nil {
		return Item{}, err
	}
	now := nowString()
	tx, err := s.db.Begin()
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE items SET reminder_at=?, version=version+1, updated_at=? WHERE id=?", remindAt, now, id)
	if err != nil {
		return Item{}, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return Item{}, sql.ErrNoRows
	}
	if _, err := tx.Exec("UPDATE reminders SET status='cancelled', updated_at=? WHERE item_id=? AND status='pending'", now, id); err != nil {
		return Item{}, err
	}
	_, err = tx.Exec("INSERT INTO reminders(id, item_id, remind_at, status, created_at, updated_at) VALUES(?, ?, ?, 'pending', ?, ?)", newID("reminder"), id, remindAt, now, now)
	if err != nil {
		return Item{}, err
	}
	if err := recordItemEventTx(tx, id, "snoozed", map[string]any{"before": before.ReminderAt, "after": remindAt}); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return s.getItem(id)
}

func (s *Store) dueReminders(limit int) ([]ReminderDelivery, error) {
	rows, err := s.db.Query(`SELECT r.id, r.item_id, i.title, r.remind_at FROM reminders r JOIN items i ON i.id=r.item_id WHERE r.status='pending' AND r.remind_at<=? AND i.status NOT IN ('done','cancelled') ORDER BY r.remind_at LIMIT ?`, nowString(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ReminderDelivery, 0)
	for rows.Next() {
		var value ReminderDelivery
		if err := rows.Scan(&value.ReminderID, &value.ItemID, &value.Title, &value.RemindAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) markReminder(id string, delivered bool, message string) error {
	status := "pending"
	sentAt := ""
	if delivered {
		status, sentAt = "sent", nowString()
	}
	_, err := s.db.Exec("UPDATE reminders SET status=CASE WHEN ?='sent' THEN 'sent' WHEN attempts+1>=5 THEN 'failed' ELSE 'pending' END, attempts=attempts+1, sent_at=?, last_error=?, updated_at=? WHERE id=?", status, sentAt, message, nowString(), id)
	return err
}

func (s *Store) savePushSubscription(value PushSubscription) error {
	now := nowString()
	_, err := s.db.Exec(`INSERT INTO push_subscriptions(endpoint, p256dh, auth, user_agent, active, created_at, updated_at) VALUES(?, ?, ?, ?, 1, ?, ?)
ON CONFLICT(endpoint) DO UPDATE SET p256dh=excluded.p256dh, auth=excluded.auth, user_agent=excluded.user_agent, active=1, updated_at=excluded.updated_at`, value.Endpoint, value.P256DH, value.Auth, value.UserAgent, now, now)
	return err
}

func (s *Store) listPushSubscriptions() ([]PushSubscription, error) {
	rows, err := s.db.Query("SELECT endpoint, p256dh, auth, user_agent FROM push_subscriptions WHERE active=1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]PushSubscription, 0)
	for rows.Next() {
		var value PushSubscription
		if err := rows.Scan(&value.Endpoint, &value.P256DH, &value.Auth, &value.UserAgent); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) deactivatePushSubscription(endpoint string) error {
	_, err := s.db.Exec("UPDATE push_subscriptions SET active=0, updated_at=? WHERE endpoint=?", nowString(), endpoint)
	return err
}

func (s *Store) exportData() (map[string]any, error) {
	captures, err := s.listCaptures(10000)
	if err != nil {
		return nil, err
	}
	items, err := s.listItems("", 10000)
	if err != nil {
		return nil, err
	}
	projects, err := s.listProjects()
	if err != nil {
		return nil, err
	}
	events, err := s.listItemEvents("", 100000)
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": 2, "exportedAt": nowString(), "captures": captures, "items": items, "projects": projects, "events": events}, nil
}

func (s *Store) backup(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "organizer-"+time.Now().UTC().Format("20060102-150405")+".db")
	escaped := strings.ReplaceAll(path, "'", "''")
	if _, err := s.db.Exec("VACUUM INTO '" + escaped + "'"); err != nil {
		return "", err
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "organizer-*.db"))
	sort.Strings(entries)
	for len(entries) > 14 {
		_ = os.Remove(entries[0])
		entries = entries[1:]
	}
	return path, nil
}
