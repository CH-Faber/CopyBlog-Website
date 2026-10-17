package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMigratedTablesHaveWorkspaceColumns(t *testing.T) {
	store := testStore(t)
	for _, table := range []string{"captures", "items", "projects", "memories", "correction_events", "push_subscriptions", "device_tokens"} {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name='workspace_id'", table).Scan(&count); err != nil {
			t.Fatalf("inspect %s columns: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("%s does not have one workspace_id column", table)
		}
	}
}

func TestAgendaRouteUnknownPath(t *testing.T) {
	_, handler, _ := testServer(t)
	for _, prefix := range []string{"/api/organizer/v1", "/api/agenda/v1"} {
		req := httptest.NewRequest(http.MethodGet, prefix+"/does-not-exist", nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %s, got %d", prefix, res.Code)
		}
	}
}

func TestAgendaAndOrganizerRouteAliases(t *testing.T) {
	_, handler, password := testServer(t)
	for _, prefix := range []string{"/api/organizer/v1", "/api/agenda/v1"} {
		login := requestJSON(t, handler, http.MethodPost, prefix+"/auth/login", map[string]string{"password": password}, nil)
		if login.Code != http.StatusOK {
			t.Fatalf("login through %s failed: %d %s", prefix, login.Code, login.Body.String())
		}
	}
}
