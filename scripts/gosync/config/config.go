package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	S3Endpoint     string
	S3Region       string
	S3AccessKey    string
	S3SecretKey    string
	S3BucketName   string
	S3Prefix       string
	AIApiKey       string
	AIBaseURL      string
	AIModel        string
	WebhookSecret  string
	LocalPostsDir  string
	ProjectRootDir string
}

func LoadConfig() *Config {
	rootDir := "."
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		rootDir = "../.."
	}

	envPath := filepath.Join(rootDir, ".env")
	err := godotenv.Load(envPath)
	if err != nil {
		log.Println("Note: .env file not loaded, using system environment variables")
	}

	return &Config{
		S3Endpoint:     GetEnvOrDefault("S3_ENDPOINT", "https://s3.cn-north-1.qiniucs.com"),
		S3Region:       GetEnvOrDefault("S3_REGION", "cn-north-1"),
		S3AccessKey:    os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:    os.Getenv("S3_SECRET_KEY"),
		S3BucketName:   os.Getenv("S3_BUCKET_NAME"),
		S3Prefix:       GetEnvOrDefault("S3_PREFIX", "website/"),
		AIApiKey:       os.Getenv("AI_API_KEY"),
		AIBaseURL:      GetEnvOrDefault("AI_BASE_URL", "https://api.openai.com/v1"),
		AIModel:        GetEnvOrDefault("AI_MODEL", "gpt-4o-mini"),
		WebhookSecret:  os.Getenv("WEBHOOK_SECRET"),
		LocalPostsDir:  filepath.Join(rootDir, "src", "content", "posts"),
		ProjectRootDir: rootDir,
	}
}

func GetEnvOrDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
