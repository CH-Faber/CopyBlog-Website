package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolateConfigEnvironment(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, key := range []string{
		"AGENDA_HOST", "AGENDA_PORT", "AGENDA_DB_PATH", "AGENDA_ATTACHMENT_DIR", "AGENDA_BACKUP_DIR",
		"AGENDA_PUBLIC_ORIGIN", "AGENDA_ADMIN_PASSWORD_HASH", "AGENDA_TIMEZONE", "AGENDA_SESSION_DAYS",
		"AGENDA_MAX_UPLOAD_MB", "ORGANIZER_HOST", "ORGANIZER_PORT", "ORGANIZER_DB_PATH",
		"ORGANIZER_ATTACHMENT_DIR", "ORGANIZER_BACKUP_DIR", "ORGANIZER_PUBLIC_ORIGIN",
		"ORGANIZER_ADMIN_PASSWORD_HASH", "ORGANIZER_TIMEZONE", "ORGANIZER_SESSION_DAYS",
		"ORGANIZER_MAX_UPLOAD_MB",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("ORGANIZER_ADMIN_PASSWORD_HASH", "$2a$10$7EqJtq98hPqEX7fNZaFWoO5Jw8QJQf0K1M8M3M6w5R0v3M8u7M3uK")
	t.Setenv("ORGANIZER_DB_PATH", filepath.Join(dir, "agenda.db"))
	t.Setenv("ORGANIZER_ATTACHMENT_DIR", filepath.Join(dir, "attachments"))
	t.Setenv("ORGANIZER_BACKUP_DIR", filepath.Join(dir, "backups"))
	t.Setenv("ORGANIZER_TIMEZONE", "UTC")
	return dir
}

func TestEnvCompat(t *testing.T) {
	t.Setenv("AGENDA_TEST_KEY", "  agenda-value  ")
	t.Setenv("ORGANIZER_TEST_KEY", "organizer-value")
	if got := envCompat("AGENDA_TEST_KEY", "ORGANIZER_TEST_KEY", "fallback"); got != "agenda-value" {
		t.Fatalf("expected trimmed Agenda value, got %q", got)
	}

	t.Setenv("AGENDA_TEST_KEY", "   ")
	if got := envCompat("AGENDA_TEST_KEY", "ORGANIZER_TEST_KEY", "fallback"); got != "organizer-value" {
		t.Fatalf("expected trimmed Organizer fallback, got %q", got)
	}

	t.Setenv("ORGANIZER_TEST_KEY", "\t")
	if got := envCompat("AGENDA_TEST_KEY", "ORGANIZER_TEST_KEY", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback when both keys are empty, got %q", got)
	}
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

func TestLoadConfigAgendaKeysWinAcrossSettings(t *testing.T) {
	dir := isolateConfigEnvironment(t)
	agendaDB := filepath.Join(dir, "agenda", "agenda.db")
	agendaAttachments := filepath.Join(dir, "agenda", "attachments")
	agendaBackups := filepath.Join(dir, "agenda", "backups")
	t.Setenv("AGENDA_HOST", "0.0.0.0")
	t.Setenv("ORGANIZER_HOST", "127.0.0.2")
	t.Setenv("AGENDA_PORT", "4111")
	t.Setenv("ORGANIZER_PORT", "4222")
	t.Setenv("AGENDA_DB_PATH", agendaDB)
	t.Setenv("ORGANIZER_DB_PATH", filepath.Join(dir, "legacy.db"))
	t.Setenv("AGENDA_ATTACHMENT_DIR", agendaAttachments)
	t.Setenv("ORGANIZER_ATTACHMENT_DIR", filepath.Join(dir, "legacy-attachments"))
	t.Setenv("AGENDA_BACKUP_DIR", agendaBackups)
	t.Setenv("ORGANIZER_BACKUP_DIR", filepath.Join(dir, "legacy-backups"))
	t.Setenv("AGENDA_PUBLIC_ORIGIN", "https://agenda.example/")
	t.Setenv("ORGANIZER_PUBLIC_ORIGIN", "https://legacy.example/")
	t.Setenv("AGENDA_ADMIN_PASSWORD_HASH", "agenda-hash")
	t.Setenv("ORGANIZER_ADMIN_PASSWORD_HASH", "organizer-hash")
	t.Setenv("AGENDA_TIMEZONE", "UTC")
	t.Setenv("ORGANIZER_TIMEZONE", "Asia/Shanghai")
	t.Setenv("AGENDA_SESSION_DAYS", "7")
	t.Setenv("ORGANIZER_SESSION_DAYS", "8")
	t.Setenv("AGENDA_MAX_UPLOAD_MB", "9")
	t.Setenv("ORGANIZER_MAX_UPLOAD_MB", "10")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "0.0.0.0" || cfg.Port != "4111" || cfg.DatabasePath != agendaDB || cfg.AttachmentDir != agendaAttachments || cfg.BackupDir != agendaBackups {
		t.Fatalf("Agenda core settings did not win: %+v", cfg)
	}
	if cfg.PublicOrigin != "https://agenda.example" || cfg.AdminPasswordHash != "agenda-hash" || cfg.Timezone != "UTC" {
		t.Fatalf("Agenda identity settings did not win: %+v", cfg)
	}
	if cfg.SessionTTL != 7*24*time.Hour || cfg.MaxUploadBytes != 9<<20 {
		t.Fatalf("Agenda limit settings did not win: %+v", cfg)
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

func TestLoadConfigValidationMentionsAgendaNames(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		value  string
		wantIn string
	}{
		{name: "session days", key: "AGENDA_SESSION_DAYS", value: "0", wantIn: "AGENDA_SESSION_DAYS"},
		{name: "max upload", key: "AGENDA_MAX_UPLOAD_MB", value: "0", wantIn: "AGENDA_MAX_UPLOAD_MB"},
		{name: "password hash", key: "AGENDA_ADMIN_PASSWORD_HASH", value: "", wantIn: "AGENDA_ADMIN_PASSWORD_HASH"},
		{name: "timezone", key: "AGENDA_TIMEZONE", value: "Not/AZone", wantIn: "AGENDA_TIMEZONE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigEnvironment(t)
			t.Setenv(tt.key, tt.value)
			if tt.key == "AGENDA_ADMIN_PASSWORD_HASH" {
				t.Setenv("ORGANIZER_ADMIN_PASSWORD_HASH", "")
			}
			_, err := loadConfig()
			if err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Fatalf("expected validation error containing %q, got %v", tt.wantIn, err)
			}
		})
	}
}
