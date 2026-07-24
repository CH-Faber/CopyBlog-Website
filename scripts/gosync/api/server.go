package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"gosync/config"
	"gosync/jobs"
	"gosync/taxonomy"
)

type Server struct {
	cfg     *config.Config
	manager *jobs.Manager
}

func NewServer(cfg *config.Config, manager *jobs.Manager) *Server {
	return &Server{cfg: cfg, manager: manager}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/", s.authorized(s.handleV1))
}

func (s *Server) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if s.cfg.WebhookSecret == "" {
			writeError(w, http.StatusServiceUnavailable, "WEBHOOK_SECRET is required for the management API")
			return
		}
		expected := "Bearer " + s.cfg.WebhookSecret
		actual := r.Header.Get("Authorization")
		if len(actual) != len(expected) || subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleV1(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	parts := strings.Split(path, "/")
	if path == "taxonomy" {
		s.handleTaxonomy(w, r)
		return
	}
	if len(parts) == 1 && parts[0] == "jobs" && r.Method == http.MethodPost {
		var request jobs.CreateRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		job, err := s.manager.Create(request)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	if len(parts) < 2 || parts[0] != "jobs" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	jobID := parts[1]
	if len(parts) == 2 && r.Method == http.MethodGet {
		job, ok := s.manager.Get(jobID)
		if !ok {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	if len(parts) == 3 && parts[2] == "articles" && r.Method == http.MethodGet {
		job, ok := s.manager.Get(jobID)
		if !ok {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeJSON(w, http.StatusOK, job.Articles)
		return
	}
	if len(parts) == 4 && parts[2] == "articles" {
		articleID := parts[3]
		if r.Method == http.MethodGet {
			article, ok := s.manager.GetArticle(jobID, articleID)
			if !ok {
				writeError(w, http.StatusNotFound, "article not found")
				return
			}
			writeJSON(w, http.StatusOK, article)
			return
		}
		if r.Method == http.MethodPut {
			var request jobs.UpdateArticleRequest
			if err := decodeJSON(r, &request); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			article, err := s.manager.UpdateArticle(jobID, articleID, request)
			if errors.Is(err, jobs.ErrConflict) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusNotFound, "article not found")
				return
			}
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, article)
			return
		}
	}
	if len(parts) == 3 && parts[2] == "publish" && r.Method == http.MethodPost {
		var request jobs.PublishRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := s.manager.Publish(jobID, request)
		if errors.Is(err, jobs.ErrConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleTaxonomy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		value, err := taxonomy.Load(s.cfg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		var value taxonomy.Taxonomy
		if err := decodeJSON(r, &value); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := taxonomy.SaveDraft(s.cfg, &value); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, &value)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func decodeJSON(r *http.Request, target interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 8<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}
