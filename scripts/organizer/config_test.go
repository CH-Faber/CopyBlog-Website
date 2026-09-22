package main

import (
	"path/filepath"
	"testing"
)

func isolateConfigEnvironment(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ORGANIZER_ADMIN_PASSWORD_HASH", "$2a$10$7EqJtq98hPqEX7fNZaFWoO5Jw8QJQf0K1M8M3M6w5R0v3M8u7M3uK")
	t.Setenv("ORGANIZER_DB_PATH", filepath.Join(dir, "agenda.db"))
	t.Setenv("ORGANIZER_ATTACHMENT_DIR", filepath.Join(dir, "attachments"))
	t.Setenv("ORGANIZER_BACKUP_DIR", filepath.Join(dir, "backups"))
	t.Setenv("ORGANIZER_TIMEZONE", "UTC")
	t.Setenv("ORGANIZER_HOST", "")
	return dir
}

func TestLoadConfigPrefersAgendaKeys(t *testing.T) {
	isolateConfigEnvironment(t)
	t.Setenv("AGENDA_PORT", "4111")
	t.Setenv("ORGANIZER_PORT", "4222")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "4111" {
		t.Fatalf("expected AGENDA_PORT to take precedence, got %q", cfg.Port)
	}
}

func TestLoadConfigFallsBackToOrganizerKeys(t *testing.T) {
	isolateConfigEnvironment(t)
	t.Setenv("AGENDA_PORT", "")
	t.Setenv("ORGANIZER_PORT", "4333")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "4333" {
		t.Fatalf("expected ORGANIZER_PORT fallback, got %q", cfg.Port)
	}
}
