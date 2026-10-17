package main

import "net/http"

// registerAgendaRoutes mounts the shared Agenda v1 handlers under a caller-provided
// prefix. Keeping registration in one place ensures legacy Organizer clients and
// the independent Agenda namespace expose identical behavior.
func (s *Server) registerAgendaRoutes(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/health", s.handleHealth)
	mux.HandleFunc("GET "+prefix+"/healthz", s.handleHealthZ)
	mux.HandleFunc("GET "+prefix+"/readyz", s.handleReadyZ)
	mux.HandleFunc("GET "+prefix+"/version", s.handleVersion)
	mux.HandleFunc("POST "+prefix+"/auth/login", s.handleLogin)
	mux.HandleFunc("POST "+prefix+"/auth/device", s.handleDeviceLogin)
	mux.HandleFunc("POST "+prefix+"/auth/logout", s.authorized(s.handleLogout))
	mux.HandleFunc("GET "+prefix+"/session", s.authorized(s.handleSession))
	mux.HandleFunc("PUT "+prefix+"/password", s.authorized(s.handleChangePassword))
	mux.HandleFunc("POST "+prefix+"/device-tokens", s.authorized(s.handleCreateDeviceToken))
	mux.HandleFunc("POST "+prefix+"/captures", s.authorized(s.handleCreateCapture))
	mux.HandleFunc("GET "+prefix+"/captures", s.authorized(s.handleListCaptures))
	mux.HandleFunc("GET "+prefix+"/captures/{id}", s.authorized(s.handleGetCapture))
	mux.HandleFunc("DELETE "+prefix+"/captures/{id}", s.authorized(s.handleDeleteCapture))
	mux.HandleFunc("GET "+prefix+"/captures/{id}/attachment", s.authorized(s.handleCaptureAttachment))
	mux.HandleFunc("POST "+prefix+"/captures/{id}/parse", s.authorized(s.handleParseCapture))
	mux.HandleFunc("POST "+prefix+"/captures/{id}/confirm", s.authorized(s.handleConfirmCapture))
	mux.HandleFunc("GET "+prefix+"/items", s.authorized(s.handleListItems))
	mux.HandleFunc("POST "+prefix+"/items", s.authorized(s.handleCreateItem))
	mux.HandleFunc("GET "+prefix+"/items/{id}", s.authorized(s.handleGetItem))
	mux.HandleFunc("PUT "+prefix+"/items/{id}", s.authorized(s.handleUpdateItem))
	mux.HandleFunc("POST "+prefix+"/items/{id}/complete", s.authorized(s.handleCompleteItem))
	mux.HandleFunc("POST "+prefix+"/items/{id}/cancel", s.authorized(s.handleCancelItem))
	mux.HandleFunc("POST "+prefix+"/items/{id}/reopen", s.authorized(s.handleReopenItem))
	mux.HandleFunc("POST "+prefix+"/items/{id}/archive", s.authorized(s.handleArchiveItem))
	mux.HandleFunc("GET "+prefix+"/items/{id}/events", s.authorized(s.handleListItemEvents))
	mux.HandleFunc("POST "+prefix+"/items/{id}/snooze", s.authorized(s.handleSnoozeItem))
	mux.HandleFunc("GET "+prefix+"/projects", s.authorized(s.handleListProjects))
	mux.HandleFunc("POST "+prefix+"/projects", s.authorized(s.handleCreateProject))
	mux.HandleFunc("GET "+prefix+"/projects/{id}", s.authorized(s.handleGetProject))
	mux.HandleFunc("PUT "+prefix+"/projects/{id}", s.authorized(s.handleUpdateProject))
	mux.HandleFunc("GET "+prefix+"/events", s.authorized(s.handleListEvents))
	mux.HandleFunc("GET "+prefix+"/memories", s.authorized(s.handleListMemories))
	mux.HandleFunc("POST "+prefix+"/memories", s.authorized(s.handleCreateMemory))
	mux.HandleFunc("PUT "+prefix+"/memories/{id}", s.authorized(s.handleUpdateMemory))
	mux.HandleFunc("GET "+prefix+"/push/vapid-key", s.authorized(s.handleVAPIDKey))
	mux.HandleFunc("POST "+prefix+"/push/subscriptions", s.authorized(s.handlePushSubscription))
	mux.HandleFunc("GET "+prefix+"/export", s.authorized(s.handleExport))
}
