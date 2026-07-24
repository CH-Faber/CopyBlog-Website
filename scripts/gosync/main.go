package main

import (
	"encoding/json"
	"log"
	"net/http"

	"gosync/ai"
	apiServer "gosync/api"
	"gosync/config"
	"gosync/jobs"
	"gosync/s3sync"
)

type StatusResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func main() {
	cfg := config.LoadConfig()

	syncer, err := s3sync.NewS3Syncer(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize S3 syncer: %v", err)
	}

	aiGenerator := ai.NewGenerator(cfg)
	if cfg.AIApiKey != "" {
		log.Printf("AI: 使用 BaseURL=%s Model=%s（密钥已配置）\n", cfg.AIBaseURL, cfg.AIModel)
	}
	jobManager, err := jobs.NewManager(cfg, syncer, aiGenerator)
	if err != nil {
		log.Fatalf("Failed to initialize job manager: %v", err)
	}
	mux := http.NewServeMux()
	apiServer.NewServer(cfg, jobManager).Register(mux)

	mux.HandleFunc("/api/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if cfg.WebhookSecret != "" {
			reqToken := r.Header.Get("Authorization")
			expected := "Bearer " + cfg.WebhookSecret
			if reqToken == "" || reqToken != expected {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		job, err := jobManager.Create(jobs.CreateRequest{ClientID: "legacy-obsidian-plugin"})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			JobID   string `json:"jobId"`
		}{
			Status: "accepted", Message: "Sync task is waiting for review and will not publish automatically.", JobID: job.ID,
		})
	})

	port := config.GetEnvOrDefault("PORT", "3001")
	log.Printf("Sync API server is listening on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
