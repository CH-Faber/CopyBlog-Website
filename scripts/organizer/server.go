package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	cfg      Config
	store    *Store
	ai       *AIClient
	executor *AssistantExecutor
	push     *PushSender
	limiter  *loginLimiter
}

const maxCaptureAttachments = 10

func newServer(cfg Config, store *Store) *Server {
	return &Server{cfg: cfg, store: store, ai: newAIClient(cfg), executor: NewAssistantExecutor(store), push: newPushSender(cfg, store), limiter: newLoginLimiter()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerAgendaRoutes(mux, "/api/organizer/v1")
	s.registerAgendaRoutes(mux, "/api/agenda/v1")
	return s.commonHeaders(mux)
}

func (s *Server) commonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		origin := strings.TrimRight(r.Header.Get("Origin"), "/")
		// Capacitor has used both http and https localhost origins across
		// Android WebView versions. These are local app origins, not public
		// origins, and native requests authenticate with a bearer token.
		if origin == "https://localhost" || origin == "http://localhost" || origin == "capacitor://localhost" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "pushEnabled": s.push.enabled(), "time": nowString()})
}

func (s *Server) handleHealthZ(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyZ(w http.ResponseWriter, _ *http.Request) {
	if err := s.store.db.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"service": "agenda-api", "version": "dev", "schemaVersion": 1})
}

func remoteIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	key := remoteIP(r)
	if !s.limiter.allow(key) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	var request struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request, 32<<10); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := s.store.adminPasswordHash(s.cfg.AdminPasswordHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load credentials")
		return
	}
	if !verifyPassword(hash, request.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.limiter.clear(key)
	token, err := newSecret(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create session")
		return
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.createSession(token, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "save session")
		return
	}
	s.setSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "expiresAt": expires.UTC().Format(time.RFC3339)})
}

func (s *Server) handleDeviceLogin(w http.ResponseWriter, r *http.Request) {
	key := remoteIP(r)
	if !s.limiter.allow(key) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	var request struct {
		Password   string `json:"password"`
		DeviceName string `json:"deviceName"`
	}
	if err := decodeJSON(r, &request, 32<<10); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := s.store.adminPasswordHash(s.cfg.AdminPasswordHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load credentials")
		return
	}
	if !verifyPassword(hash, request.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	name := strings.TrimSpace(request.DeviceName)
	if name == "" {
		name = "Agenda Android"
	}
	if len([]rune(name)) > 80 {
		writeError(w, http.StatusBadRequest, "device name is too long")
		return
	}
	s.limiter.clear(key)
	id, token, err := s.store.createDeviceToken(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create device token")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"authenticated": true, "deviceId": id, "token": token})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := bearerToken(r); token != "" {
		if err := s.store.revokeDeviceToken(token); err != nil {
			writeError(w, http.StatusInternalServerError, "revoke device token")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.deleteSession(cookie.Value)
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) handleSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "pushEnabled": s.push.enabled(), "timezone": s.cfg.Timezone})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var request struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeJSON(r, &request, 32<<10); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(request.NewPassword) < 12 {
		writeError(w, http.StatusBadRequest, "new password must contain at least 12 characters")
		return
	}
	currentHash, err := s.store.adminPasswordHash(s.cfg.AdminPasswordHash)
	if err != nil || !verifyPassword(currentHash, request.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	hash, err := hashPassword(request.NewPassword)
	if err != nil || s.store.setAdminPasswordHash(hash) != nil {
		writeError(w, http.StatusInternalServerError, "update password")
		return
	}
	_ = s.store.revokeAllSessions()
	token, err := newSecret(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create session")
		return
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.createSession(token, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "save session")
		return
	}
	s.setSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) handleCreateDeviceToken(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &request, 32<<10); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		request.Name = "Obsidian"
	}
	id, token, err := s.store.createDeviceToken(request.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create device token")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "token": token, "message": "This token is shown only once."})
}

func (s *Server) handleCreateCapture(w http.ResponseWriter, r *http.Request) {
	capture, err := s.captureFromRequest(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.store.createCapture(capture)
	if err != nil {
		removeCaptureAttachmentFiles(capture)
		writeError(w, http.StatusInternalServerError, "save capture")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) captureFromRequest(w http.ResponseWriter, r *http.Request) (Capture, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes*maxCaptureAttachments+(1<<20))
		if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
			return Capture{}, fmt.Errorf("invalid multipart request: %w", err)
		}
		defer r.MultipartForm.RemoveAll()
		capture := Capture{ID: newID("capture"), SourceType: strings.TrimSpace(r.FormValue("sourceType")), RawText: strings.TrimSpace(r.FormValue("rawText"))}
		headers := append([]*multipart.FileHeader{}, r.MultipartForm.File["attachments"]...)
		headers = append(headers, r.MultipartForm.File["attachment"]...)
		if len(headers) > maxCaptureAttachments {
			return Capture{}, fmt.Errorf("at most %d images are allowed", maxCaptureAttachments)
		}
		for ordinal, header := range headers {
			file, err := header.Open()
			if err != nil {
				removeCaptureAttachmentFiles(capture)
				return Capture{}, err
			}
			err = s.saveCaptureAttachment(&capture, file, header, ordinal)
			closeErr := file.Close()
			if err != nil {
				removeCaptureAttachmentFiles(capture)
				return Capture{}, err
			}
			if closeErr != nil {
				removeCaptureAttachmentFiles(capture)
				return Capture{}, closeErr
			}
		}
		if capture.SourceType == "" {
			if len(capture.Attachments) > 0 {
				capture.SourceType = "image"
			} else {
				capture.SourceType = "text"
			}
		}
		if capture.RawText == "" && len(capture.Attachments) == 0 {
			return Capture{}, errors.New("text or attachment is required")
		}
		return capture, nil
	}
	var request struct {
		RawText    string `json:"rawText"`
		SourceType string `json:"sourceType"`
	}
	if err := decodeJSON(r, &request, 1<<20); err != nil {
		return Capture{}, err
	}
	request.RawText = strings.TrimSpace(request.RawText)
	if request.RawText == "" {
		return Capture{}, errors.New("rawText is required")
	}
	if request.SourceType == "" {
		request.SourceType = "text"
	}
	return Capture{ID: newID("capture"), SourceType: request.SourceType, RawText: request.RawText}, nil
}

func (s *Server) saveCaptureAttachment(capture *Capture, file multipart.File, header *multipart.FileHeader, ordinal int) error {
	mime := header.Header.Get("Content-Type")
	extensions := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif"}
	extension, ok := extensions[mime]
	if !ok {
		return errors.New("attachment must be PNG, JPEG, WebP, or GIF")
	}
	path := filepath.Join(s.cfg.AttachmentDir, fmt.Sprintf("%s-%d%s", capture.ID, ordinal, extension))
	target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer target.Close()
	written, err := io.Copy(target, io.LimitReader(file, s.cfg.MaxUploadBytes+1))
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	if written > s.cfg.MaxUploadBytes {
		_ = os.Remove(path)
		return errors.New("attachment is too large")
	}
	attachment := CaptureAttachment{Name: filepath.Base(header.Filename), Mime: mime, Path: path}
	capture.Attachments = append(capture.Attachments, attachment)
	if len(capture.Attachments) == 1 {
		capture.AttachmentName = attachment.Name
		capture.AttachmentMime = attachment.Mime
		capture.AttachmentPath = attachment.Path
	}
	return nil
}

func removeCaptureAttachmentFiles(capture Capture) {
	for _, attachment := range captureAttachmentValues(capture) {
		if attachment.Path != "" {
			_ = os.Remove(attachment.Path)
		}
	}
}

func captureAttachmentValues(capture Capture) []CaptureAttachment {
	if len(capture.Attachments) > 0 {
		return capture.Attachments
	}
	if capture.AttachmentPath != "" {
		return []CaptureAttachment{{Name: capture.AttachmentName, Mime: capture.AttachmentMime, Path: capture.AttachmentPath}}
	}
	return nil
}

func (s *Server) handleListCaptures(w http.ResponseWriter, r *http.Request) {
	limit := queryLimit(r, 100, 500)
	values, err := s.store.listCaptures(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list captures")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"captures": values})
}

func (s *Server) handleGetCapture(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.getCapture(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get capture")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) handleDeleteCapture(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.getCapture(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get capture")
		return
	}

	type stagedFile struct{ original, staged string }
	stagedAttachments := make([]stagedFile, 0, len(value.Attachments))
	for index, attachment := range captureAttachmentValues(value) {
		path, ok := attachmentPathWithin(s.cfg.AttachmentDir, attachment.Path)
		if !ok {
			for _, staged := range stagedAttachments {
				_ = os.Rename(staged.staged, staged.original)
			}
			writeError(w, http.StatusInternalServerError, "invalid attachment path")
			return
		}
		staged := path + ".deleting-" + value.ID + "-" + strconv.Itoa(index)
		if err := os.Rename(path, staged); err != nil && !errors.Is(err, os.ErrNotExist) {
			for _, previous := range stagedAttachments {
				_ = os.Rename(previous.staged, previous.original)
			}
			writeError(w, http.StatusInternalServerError, "remove attachment")
			return
		}
		stagedAttachments = append(stagedAttachments, stagedFile{original: path, staged: staged})
	}

	if err := s.store.deleteCapture(value.ID); err != nil {
		for _, staged := range stagedAttachments {
			_ = os.Rename(staged.staged, staged.original)
		}
		writeError(w, http.StatusInternalServerError, "delete capture")
		return
	}
	for _, staged := range stagedAttachments {
		_ = os.Remove(staged.staged)
	}
	w.WriteHeader(http.StatusNoContent)
}

func attachmentPathWithin(root, value string) (string, bool) {
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(rootPath, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}

func (s *Server) handleCaptureAttachment(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.getCapture(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	index := 0
	if rawIndex := r.PathValue("index"); rawIndex != "" {
		index, err = strconv.Atoi(rawIndex)
	}
	attachments := captureAttachmentValues(value)
	if err != nil || index < 0 || index >= len(attachments) {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	attachment := attachments[index]
	file, err := os.Open(attachment.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read attachment")
		return
	}
	w.Header().Set("Content-Type", attachment.Mime)
	w.Header().Set("Content-Disposition", `inline; filename="attachment"`)
	http.ServeContent(w, r, attachment.Name, info.ModTime(), file)
}

func (s *Server) handleParseCapture(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	capture, err := s.store.getCapture(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get capture")
		return
	}
	if capture.Status == "confirmed" {
		writeError(w, http.StatusConflict, "capture is already confirmed")
		return
	}
	_ = s.store.setCaptureParsing(id)
	memoryPrompt, err := s.store.activeMemoryPrompt()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load assistant memory")
		return
	}
	result, err := s.ai.Parse(r.Context(), capture, memoryPrompt)
	if err != nil {
		_ = s.store.setCaptureError(id, err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := s.store.setCaptureResult(id, result); err != nil {
		writeError(w, http.StatusInternalServerError, "save parse result")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleConfirmCapture(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	capture, err := s.store.getCapture(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get capture")
		return
	}
	if capture.Status == "confirmed" {
		writeError(w, http.StatusConflict, "capture is already confirmed")
		return
	}
	var request ParseResult
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &request, 1<<20); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if len(capture.AIResult) > 0 {
		if err := json.Unmarshal(capture.AIResult, &request); err != nil {
			writeError(w, http.StatusBadRequest, "capture has an invalid parse result")
			return
		}
	}
	applyCaptureInvariants(capture, &request, s.cfg.Timezone)
	if err := validateCandidates(request.Items, s.cfg.Timezone); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.recordCaptureCorrections(capture, request, s.cfg.Timezone); err != nil {
		writeError(w, http.StatusInternalServerError, "save corrections")
		return
	}
	items, err := s.store.createItems(id, request.Items, s.cfg.Timezone)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "confirm capture")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": items})
}

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	values, err := s.store.listItems(status, queryLimit(r, 250, 1000))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list items")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	var candidate Candidate
	if err := decodeJSON(r, &candidate, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateCandidates([]Candidate{candidate}, s.cfg.Timezone); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := s.store.createItems("", []Candidate{candidate}, s.cfg.Timezone)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create item")
		return
	}
	writeJSON(w, http.StatusCreated, items[0])
}

func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.getItem(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get item")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	var item Item
	if err := decodeJSON(r, &item, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item.ID = r.PathValue("id")
	if err := validateItem(item); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.store.updateItem(item)
	if errors.Is(err, errConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update item")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleCompleteItem(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.completeItem(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "complete item")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleCancelItem(w http.ResponseWriter, r *http.Request) {
	s.handleItemTransition(w, r, s.store.cancelItem, "cancel item")
}

func (s *Server) handleReopenItem(w http.ResponseWriter, r *http.Request) {
	s.handleItemTransition(w, r, s.store.reopenItem, "reopen item")
}

func (s *Server) handleArchiveItem(w http.ResponseWriter, r *http.Request) {
	s.handleItemTransition(w, r, s.store.archiveItem, "archive item")
}

func (s *Server) handleItemTransition(w http.ResponseWriter, r *http.Request, action func(string) (Item, error), message string) {
	item, err := action(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, message)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleListItemEvents(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.listItemEvents(r.PathValue("id"), queryLimit(r, 100, 500))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list item events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": values})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.listItemEvents("", queryLimit(r, 250, 1000))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": values})
}

func (s *Server) handleListProjects(w http.ResponseWriter, _ *http.Request) {
	values, err := s.store.listProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list projects")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": values})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var project Project
	if err := decodeJSON(r, &project, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.store.createProject(project)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.store.getProject(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get project")
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	var project Project
	if err := decodeJSON(r, &project, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	project.ID = r.PathValue("id")
	updated, err := s.store.updateProject(project)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if errors.Is(err, errConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.listMemories(strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list memories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": values})
}

func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	var memory Memory
	if err := decodeJSON(r, &memory, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.store.createMemory(memory)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateMemory(w http.ResponseWriter, r *http.Request) {
	var memory Memory
	if err := decodeJSON(r, &memory, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	memory.ID = r.PathValue("id")
	updated, err := s.store.updateMemory(memory)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleSnoozeItem(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RemindAt string `json:"remindAt"`
		Minutes  int    `json:"minutes"`
	}
	if err := decodeJSON(r, &request, 32<<10); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.RemindAt == "" {
		if request.Minutes <= 0 || request.Minutes > 43200 {
			request.Minutes = 10
		}
		request.RemindAt = time.Now().Add(time.Duration(request.Minutes) * time.Minute).UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, request.RemindAt); err != nil {
		writeError(w, http.StatusBadRequest, "remindAt must be RFC3339")
		return
	}
	item, err := s.store.snoozeItem(r.PathValue("id"), request.RemindAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "snooze item")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleVAPIDKey(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"enabled": s.push.enabled(), "publicKey": s.cfg.VAPIDPublicKey})
}

func (s *Server) handlePushSubscription(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256DH string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := decodeJSON(r, &request, 128<<10); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.Endpoint == "" || request.Keys.P256DH == "" || request.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "invalid push subscription")
		return
	}
	err := s.store.savePushSubscription(PushSubscription{Endpoint: request.Endpoint, P256DH: request.Keys.P256DH, Auth: request.Keys.Auth, UserAgent: r.UserAgent()})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save push subscription")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"subscribed": true})
}

func (s *Server) handleExport(w http.ResponseWriter, _ *http.Request) {
	data, err := s.store.exportData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export data")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="faber-organizer-export.json"`)
	writeJSON(w, http.StatusOK, data)
}

func queryLimit(r *http.Request, fallback, maximum int) int {
	value, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || value < 1 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func validateCandidates(values []Candidate, defaultTimezone string) error {
	if len(values) == 0 || len(values) > 25 {
		return errors.New("between 1 and 25 items are required")
	}
	for index, value := range values {
		value = normalizeCandidate(value, defaultTimezone)
		if value.Type != "task" && value.Type != "event" && value.Type != "reminder" && value.Type != "note" {
			return fmt.Errorf("item %d has invalid type", index+1)
		}
		if value.Title == "" {
			return fmt.Errorf("item %d title is required", index+1)
		}
		if len(value.Title) > 240 || len(value.Description) > 10000 {
			return fmt.Errorf("item %d is too long", index+1)
		}
		if _, err := time.LoadLocation(value.Timezone); err != nil {
			return fmt.Errorf("item %d has invalid timezone", index+1)
		}
		for _, pair := range []struct{ name, value string }{{"startAt", value.StartAt}, {"endAt", value.EndAt}, {"dueAt", value.DueAt}, {"reminderAt", value.ReminderAt}, {"availableFrom", value.AvailableFrom}, {"availableUntil", value.AvailableUntil}} {
			if pair.value != "" {
				if _, err := time.Parse(time.RFC3339, pair.value); err != nil {
					return fmt.Errorf("item %d %s must be RFC3339", index+1, pair.name)
				}
			}
		}
		if value.StartAt != "" && value.EndAt != "" {
			start, _ := time.Parse(time.RFC3339, value.StartAt)
			end, _ := time.Parse(time.RFC3339, value.EndAt)
			if end.Before(start) {
				return fmt.Errorf("item %d endAt must not be before startAt", index+1)
			}
		}
	}
	return nil
}

func validateItem(item Item) error {
	if item.Version < 1 {
		return errors.New("version is required")
	}
	if item.Status != "inbox" && item.Status != "todo" && item.Status != "doing" && item.Status != "done" && item.Status != "cancelled" && item.Status != "archived" {
		return errors.New("invalid status")
	}
	return validateCandidates([]Candidate{{Type: item.Type, Title: item.Title, Description: item.Description, StartAt: item.StartAt, EndAt: item.EndAt, DueAt: item.DueAt, ReminderAt: item.ReminderAt, Timezone: item.Timezone, AllDay: item.AllDay, RecurrenceRule: item.RecurrenceRule, Priority: item.Priority, Certainty: item.Certainty, DurationMinutes: item.DurationMinutes, AvailableFrom: item.AvailableFrom, AvailableUntil: item.AvailableUntil, Project: item.Project, Tags: item.Tags, Location: item.Location, People: item.People}}, item.Timezone)
}

func decodeJSON(r *http.Request, target any, limit int64) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, limit))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}
