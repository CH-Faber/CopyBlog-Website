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
);`
	_, err := s.db.Exec(schema)
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
	if candidate.Type != "event" && candidate.Type != "reminder" {
		candidate.Type = "task"
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
		var captureReference any
		if captureID != "" {
			captureReference = captureID
		}
		_, err := tx.Exec(`INSERT INTO items(id, capture_id, type, title, description, start_at, end_at, due_at, reminder_at, timezone, all_day, recurrence_rule, priority, status, project, tags_json, location, people_json, version, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'todo', ?, ?, ?, ?, 1, ?, ?)`, id, captureReference, candidate.Type, candidate.Title, candidate.Description, candidate.StartAt, candidate.EndAt, candidate.DueAt, candidate.ReminderAt, candidate.Timezone, candidate.AllDay, candidate.RecurrenceRule, candidate.Priority, candidate.Project, string(tags), candidate.Location, string(people), now, now)
		if err != nil {
			return nil, err
		}
		if candidate.ReminderAt != "" {
			_, err = tx.Exec("INSERT INTO reminders(id, item_id, remind_at, status, created_at, updated_at) VALUES(?, ?, ?, 'pending', ?, ?)", newID("reminder"), id, candidate.ReminderAt, now, now)
			if err != nil {
				return nil, err
			}
		}
		created = append(created, Item{ID: id, CaptureID: captureID, Type: candidate.Type, Title: candidate.Title, Description: candidate.Description, StartAt: candidate.StartAt, EndAt: candidate.EndAt, DueAt: candidate.DueAt, ReminderAt: candidate.ReminderAt, Timezone: candidate.Timezone, AllDay: candidate.AllDay, RecurrenceRule: candidate.RecurrenceRule, Priority: candidate.Priority, Status: "todo", Project: candidate.Project, Tags: candidate.Tags, Location: candidate.Location, People: candidate.People, Version: 1, CreatedAt: now, UpdatedAt: now})
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

const itemColumns = "id, COALESCE(capture_id, ''), type, title, description, start_at, end_at, due_at, reminder_at, timezone, all_day, recurrence_rule, priority, status, project, tags_json, location, people_json, version, created_at, updated_at"

func scanItem(scanner interface{ Scan(...any) error }) (Item, error) {
	var item Item
	var allDay int
	var tags, people string
	err := scanner.Scan(&item.ID, &item.CaptureID, &item.Type, &item.Title, &item.Description, &item.StartAt, &item.EndAt, &item.DueAt, &item.ReminderAt, &item.Timezone, &allDay, &item.RecurrenceRule, &item.Priority, &item.Status, &item.Project, &tags, &item.Location, &people, &item.Version, &item.CreatedAt, &item.UpdatedAt)
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

func (s *Store) updateItem(item Item) (Item, error) {
	candidate := normalizeCandidate(Candidate{
		Type: item.Type, Title: item.Title, Description: item.Description, StartAt: item.StartAt,
		EndAt: item.EndAt, DueAt: item.DueAt, ReminderAt: item.ReminderAt, Timezone: item.Timezone,
		AllDay: item.AllDay, RecurrenceRule: item.RecurrenceRule, Priority: item.Priority,
		Project: item.Project, Tags: item.Tags, Location: item.Location, People: item.People,
	}, item.Timezone)
	if candidate.Title == "" {
		return Item{}, errors.New("title is required")
	}
	item.Type, item.Title, item.Description = candidate.Type, candidate.Title, candidate.Description
	item.StartAt, item.EndAt, item.DueAt, item.ReminderAt = candidate.StartAt, candidate.EndAt, candidate.DueAt, candidate.ReminderAt
	item.Timezone, item.AllDay, item.RecurrenceRule, item.Priority = candidate.Timezone, candidate.AllDay, candidate.RecurrenceRule, candidate.Priority
	item.Project, item.Tags, item.Location, item.People = candidate.Project, candidate.Tags, candidate.Location, candidate.People
	tags, _ := json.Marshal(candidate.Tags)
	people, _ := json.Marshal(candidate.People)
	now := nowString()
	tx, err := s.db.Begin()
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE items SET type=?, title=?, description=?, start_at=?, end_at=?, due_at=?, reminder_at=?, timezone=?, all_day=?, recurrence_rule=?, priority=?, status=?, project=?, tags_json=?, location=?, people_json=?, version=version+1, updated_at=? WHERE id=? AND version=?`, item.Type, item.Title, item.Description, item.StartAt, item.EndAt, item.DueAt, item.ReminderAt, item.Timezone, item.AllDay, item.RecurrenceRule, item.Priority, item.Status, item.Project, string(tags), item.Location, string(people), now, item.ID, item.Version)
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
	if item.ReminderAt != "" && item.Status != "done" && item.Status != "cancelled" {
		_, err = tx.Exec("INSERT INTO reminders(id, item_id, remind_at, status, created_at, updated_at) VALUES(?, ?, ?, 'pending', ?, ?)", newID("reminder"), item.ID, item.ReminderAt, now, now)
		if err != nil {
			return Item{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return s.getItem(item.ID)
}

func (s *Store) completeItem(id string) (Item, error) {
	now := nowString()
	result, err := s.db.Exec("UPDATE items SET status='done', version=version+1, updated_at=? WHERE id=?", now, id)
	if err != nil {
		return Item{}, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return Item{}, sql.ErrNoRows
	}
	_, _ = s.db.Exec("UPDATE reminders SET status='cancelled', updated_at=? WHERE item_id=? AND status='pending'", now, id)
	return s.getItem(id)
}

func (s *Store) snoozeItem(id string, remindAt string) (Item, error) {
	now := nowString()
	result, err := s.db.Exec("UPDATE items SET reminder_at=?, version=version+1, updated_at=? WHERE id=?", remindAt, now, id)
	if err != nil {
		return Item{}, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return Item{}, sql.ErrNoRows
	}
	_, _ = s.db.Exec("UPDATE reminders SET status='cancelled', updated_at=? WHERE item_id=? AND status='pending'", now, id)
	_, err = s.db.Exec("INSERT INTO reminders(id, item_id, remind_at, status, created_at, updated_at) VALUES(?, ?, ?, 'pending', ?, ?)", newID("reminder"), id, remindAt, now, now)
	if err != nil {
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
	return map[string]any{"version": 1, "exportedAt": nowString(), "captures": captures, "items": items}, nil
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
