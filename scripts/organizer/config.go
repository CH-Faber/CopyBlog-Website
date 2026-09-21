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
		Host:              envDefault("ORGANIZER_HOST", "127.0.0.1"),
		Port:              envDefault("ORGANIZER_PORT", "3020"),
		DatabasePath:      envDefault("ORGANIZER_DB_PATH", "/var/lib/faber-organizer/organizer.db"),
		AttachmentDir:     envDefault("ORGANIZER_ATTACHMENT_DIR", "/var/lib/faber-organizer/attachments"),
		BackupDir:         envDefault("ORGANIZER_BACKUP_DIR", "/var/lib/faber-organizer/backups"),
		PublicOrigin:      strings.TrimRight(envDefault("ORGANIZER_PUBLIC_ORIGIN", "https://faberhu.top"), "/"),
		AdminPasswordHash: strings.TrimSpace(os.Getenv("ORGANIZER_ADMIN_PASSWORD_HASH")),
		AIBaseURL:         strings.TrimRight(envDefault("AI_BASE_URL", "https://api.openai.com/v1"), "/"),
		AIAPIKey:          strings.TrimSpace(os.Getenv("AI_API_KEY")),
		AIModel:           envDefault("AI_MODEL", "gpt-4o-mini"),
		VAPIDPublicKey:    strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY")),
		VAPIDPrivateKey:   strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY")),
		VAPIDSubject:      envDefault("VAPID_SUBJECT", "mailto:admin@faberhu.top"),
		Timezone:          envDefault("ORGANIZER_TIMEZONE", "Asia/Shanghai"),
		SessionTTL:        30 * 24 * time.Hour,
		MaxUploadBytes:    8 << 20,
	}

	if raw := strings.TrimSpace(os.Getenv("ORGANIZER_SESSION_DAYS")); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 1 || days > 365 {
			return Config{}, fmt.Errorf("ORGANIZER_SESSION_DAYS must be between 1 and 365")
		}
		cfg.SessionTTL = time.Duration(days) * 24 * time.Hour
	}
	if raw := strings.TrimSpace(os.Getenv("ORGANIZER_MAX_UPLOAD_MB")); raw != "" {
		mb, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || mb < 1 || mb > 25 {
			return Config{}, fmt.Errorf("ORGANIZER_MAX_UPLOAD_MB must be between 1 and 25")
		}
		cfg.MaxUploadBytes = mb << 20
	}
	if cfg.AdminPasswordHash == "" {
		return Config{}, fmt.Errorf("ORGANIZER_ADMIN_PASSWORD_HASH is required")
	}
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return Config{}, fmt.Errorf("invalid ORGANIZER_TIMEZONE: %w", err)
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
