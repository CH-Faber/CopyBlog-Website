package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
)

func testServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	password := "test-password-with-length"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfg := Config{
		DatabasePath:      filepath.Join(dir, "organizer.db"),
		AttachmentDir:     filepath.Join(dir, "attachments"),
		BackupDir:         filepath.Join(dir, "backups"),
		PublicOrigin:      "https://faberhu.top",
		AdminPasswordHash: hash,
		Timezone:          "Asia/Shanghai",
		SessionTTL:        24 * 60 * 60 * 1e9,
		MaxUploadBytes:    8 << 20,
	}
	if err := os.MkdirAll(cfg.AttachmentDir, 0o750); err != nil {
		t.Fatal(err)
	}
	store, err := openStore(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	server := newServer(cfg, store)
	return server, server.Handler(), password
}

func bearerJSON(t *testing.T, handler http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func loginCookie(t *testing.T, handler http.Handler, password string) *http.Cookie {
	t.Helper()
	login := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/auth/login", map[string]string{"password": password}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("missing session cookie")
	}
	return cookies[0]
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		request.Header.Set("Origin", "https://faberhu.top")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestAuthenticatedCaptureFlow(t *testing.T) {
	_, handler, password := testServer(t)
	cookie := loginCookie(t, handler, password)

	created := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/captures", map[string]string{"rawText": "明天下午写报告", "sourceType": "voice_text"}, cookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("create capture failed: %d %s", created.Code, created.Body.String())
	}
	var capture Capture
	if err := json.Unmarshal(created.Body.Bytes(), &capture); err != nil {
		t.Fatal(err)
	}
	parsed := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/captures/"+capture.ID+"/parse", nil, cookie)
	if parsed.Code != http.StatusOK {
		t.Fatalf("parse failed: %d %s", parsed.Code, parsed.Body.String())
	}
	confirmed := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/captures/"+capture.ID+"/confirm", nil, cookie)
	if confirmed.Code != http.StatusCreated {
		t.Fatalf("confirm failed: %d %s", confirmed.Code, confirmed.Body.String())
	}
	duplicate := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/captures/"+capture.ID+"/confirm", nil, cookie)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate confirmation should conflict: %d %s", duplicate.Code, duplicate.Body.String())
	}
	listed := requestJSON(t, handler, http.MethodGet, "/api/organizer/v1/items", nil, cookie)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte("明天下午写报告")) {
		t.Fatalf("list failed: %d %s", listed.Code, listed.Body.String())
	}
}

func TestDeviceTokenCanUseOrganizerAPI(t *testing.T) {
	_, handler, password := testServer(t)
	cookie := loginCookie(t, handler, password)
	createdToken := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/device-tokens", map[string]string{"name": "Obsidian test"}, cookie)
	if createdToken.Code != http.StatusCreated {
		t.Fatalf("create token failed: %d %s", createdToken.Code, createdToken.Body.String())
	}
	var tokenResponse struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(createdToken.Body.Bytes(), &tokenResponse); err != nil || tokenResponse.Token == "" {
		t.Fatalf("invalid token response: %v %s", err, createdToken.Body.String())
	}
	created := bearerJSON(t, handler, http.MethodPost, "/api/organizer/v1/items", Candidate{Type: "task", Title: "来自 Obsidian"}, tokenResponse.Token)
	if created.Code != http.StatusCreated {
		t.Fatalf("bearer create failed: %d %s", created.Code, created.Body.String())
	}
	listed := bearerJSON(t, handler, http.MethodGet, "/api/organizer/v1/items", nil, tokenResponse.Token)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte("来自 Obsidian")) {
		t.Fatalf("bearer list failed: %d %s", listed.Code, listed.Body.String())
	}
}

func TestMultipartImageCapture(t *testing.T) {
	server, handler, password := testServer(t)
	cookie := loginCookie(t, handler, password)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("rawText", "截图里的会议安排")
	_ = writer.WriteField("sourceType", "image")
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="attachment"; filename="schedule.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("fake png body")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/organizer/v1/captures", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "https://faberhu.top")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !bytes.Contains(recorder.Body.Bytes(), []byte(`"hasAttachment":true`)) {
		t.Fatalf("multipart capture failed: %d %s", recorder.Code, recorder.Body.String())
	}
	var capture Capture
	if err := json.Unmarshal(recorder.Body.Bytes(), &capture); err != nil {
		t.Fatal(err)
	}
	stored, err := server.store.getCapture(capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stored.AttachmentPath); err != nil {
		t.Fatalf("attachment was not stored: %v", err)
	}

	deleted := requestJSON(t, handler, http.MethodDelete, "/api/organizer/v1/captures/"+capture.ID, nil, cookie)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete capture failed: %d %s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(stored.AttachmentPath); !os.IsNotExist(err) {
		t.Fatalf("attachment was not permanently deleted: %v", err)
	}
	missing := requestJSON(t, handler, http.MethodGet, "/api/organizer/v1/captures/"+capture.ID, nil, cookie)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted capture is still available: %d %s", missing.Code, missing.Body.String())
	}
}

func TestItemVersionConflictAndPushSubscription(t *testing.T) {
	_, handler, password := testServer(t)
	cookie := loginCookie(t, handler, password)
	created := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/items", Candidate{Type: "task", Title: "并发事项"}, cookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("create item failed: %d %s", created.Code, created.Body.String())
	}
	var item Item
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	item.Description = "第一次修改"
	updated := requestJSON(t, handler, http.MethodPut, "/api/organizer/v1/items/"+item.ID, item, cookie)
	if updated.Code != http.StatusOK {
		t.Fatalf("update failed: %d %s", updated.Code, updated.Body.String())
	}
	item.Description = "过期版本修改"
	conflict := requestJSON(t, handler, http.MethodPut, "/api/organizer/v1/items/"+item.ID, item, cookie)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got %d %s", conflict.Code, conflict.Body.String())
	}
	subscription := map[string]any{
		"endpoint": "https://push.example.test/subscription/1",
		"keys":     map[string]string{"p256dh": "test-p256dh", "auth": "test-auth"},
	}
	push := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/push/subscriptions", subscription, cookie)
	if push.Code != http.StatusCreated {
		t.Fatalf("push subscription failed: %d %s", push.Code, push.Body.String())
	}
}

func TestPasswordChangeRevokesOldPassword(t *testing.T) {
	_, handler, password := testServer(t)
	cookie := loginCookie(t, handler, password)
	newPassword := "new-test-password-with-length"
	changed := requestJSON(t, handler, http.MethodPut, "/api/organizer/v1/password", map[string]string{
		"currentPassword": password,
		"newPassword":     newPassword,
	}, cookie)
	if changed.Code != http.StatusOK {
		t.Fatalf("password change failed: %d %s", changed.Code, changed.Body.String())
	}
	oldLogin := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/auth/login", map[string]string{"password": password}, nil)
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old password still accepted: %d", oldLogin.Code)
	}
	newLogin := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/auth/login", map[string]string{"password": newPassword}, nil)
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password rejected: %d %s", newLogin.Code, newLogin.Body.String())
	}
}

func TestMutationRejectsWrongOrigin(t *testing.T) {
	_, handler, password := testServer(t)
	login := requestJSON(t, handler, http.MethodPost, "/api/organizer/v1/auth/login", map[string]string{"password": password}, nil)
	cookie := login.Result().Cookies()[0]
	request := httptest.NewRequest(http.MethodPost, "/api/organizer/v1/captures", bytes.NewBufferString(`{"rawText":"test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://attacker.example")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}
