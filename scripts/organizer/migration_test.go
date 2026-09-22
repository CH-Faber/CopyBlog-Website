package main

import (
	"net/http"
	"testing"
)

func TestAgendaAndOrganizerRouteAliases(t *testing.T) {
	_, handler, password := testServer(t)
	for _, prefix := range []string{"/api/organizer/v1", "/api/agenda/v1"} {
		login := requestJSON(t, handler, http.MethodPost, prefix+"/auth/login", map[string]string{"password": password}, nil)
		if login.Code != http.StatusOK {
			t.Fatalf("login through %s failed: %d %s", prefix, login.Code, login.Body.String())
		}
	}
}
