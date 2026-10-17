package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host              string
	Port              string
	DatabasePath      string
	AttachmentDir     string
	BackupDir         string
	PublicOrigin      string
	AdminPasswordHash string
	AIBaseURL         string
	AIAPIKey          string
	AIModel           string
	VAPIDPublicKey    string
	VAPIDPrivateKey   string
	VAPIDSubject      string
	Timezone          string
	SessionTTL        time.Duration
	MaxUploadBytes    int64
}

func loadConfig() (Config, error) {
	cfg := Config{
		Host:              envCompat("AGENDA_HOST", "ORGANIZER_HOST", "127.0.0.1"),
		Port:              envCompat("AGENDA_PORT", "ORGANIZER_PORT", "3020"),
		DatabasePath:      envCompat("AGENDA_DB_PATH", "ORGANIZER_DB_PATH", "/var/lib/faber-organizer/organizer.db"),
		AttachmentDir:     envCompat("AGENDA_ATTACHMENT_DIR", "ORGANIZER_ATTACHMENT_DIR", "/var/lib/faber-organizer/attachments"),
		BackupDir:         envCompat("AGENDA_BACKUP_DIR", "ORGANIZER_BACKUP_DIR", "/var/lib/faber-organizer/backups"),
		PublicOrigin:      strings.TrimRight(envCompat("AGENDA_PUBLIC_ORIGIN", "ORGANIZER_PUBLIC_ORIGIN", "https://faberhu.top"), "/"),
		AdminPasswordHash: envCompat("AGENDA_ADMIN_PASSWORD_HASH", "ORGANIZER_ADMIN_PASSWORD_HASH", ""),
		AIBaseURL:         strings.TrimRight(envDefault("AI_BASE_URL", "https://api.openai.com/v1"), "/"),
		AIAPIKey:          strings.TrimSpace(os.Getenv("AI_API_KEY")),
		AIModel:           envDefault("AI_MODEL", "gpt-4o-mini"),
		VAPIDPublicKey:    strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY")),
		VAPIDPrivateKey:   strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY")),
		VAPIDSubject:      envDefault("VAPID_SUBJECT", "mailto:admin@faberhu.top"),
		Timezone:          envCompat("AGENDA_TIMEZONE", "ORGANIZER_TIMEZONE", "Asia/Shanghai"),
		SessionTTL:        30 * 24 * time.Hour,
		MaxUploadBytes:    8 << 20,
	}

	if raw := envCompat("AGENDA_SESSION_DAYS", "ORGANIZER_SESSION_DAYS", ""); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 1 || days > 365 {
			return Config{}, fmt.Errorf("AGENDA_SESSION_DAYS must be between 1 and 365")
		}
		cfg.SessionTTL = time.Duration(days) * 24 * time.Hour
	}
	if raw := envCompat("AGENDA_MAX_UPLOAD_MB", "ORGANIZER_MAX_UPLOAD_MB", ""); raw != "" {
		mb, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || mb < 1 || mb > 25 {
			return Config{}, fmt.Errorf("AGENDA_MAX_UPLOAD_MB must be between 1 and 25")
		}
		cfg.MaxUploadBytes = mb << 20
	}
	if cfg.AdminPasswordHash == "" {
		return Config{}, fmt.Errorf("AGENDA_ADMIN_PASSWORD_HASH is required")
	}
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return Config{}, fmt.Errorf("invalid AGENDA_TIMEZONE: %w", err)
	}
	for _, dir := range []string{filepath.Dir(cfg.DatabasePath), cfg.AttachmentDir, cfg.BackupDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return Config{}, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return cfg, nil
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envCompat(agendaKey, legacyKey, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(agendaKey)); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv(legacyKey)); value != "" {
		return value
	}
	return fallback
}
