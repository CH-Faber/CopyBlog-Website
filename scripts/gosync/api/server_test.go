package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gosync/config"
	"gosync/jobs"
)

func TestTaxonomyAPISeedsExistingTags(t *testing.T) {
	root := t.TempDir()
	posts := filepath.Join(root, "src", "content", "posts")
	if err := os.MkdirAll(posts, 0755); err != nil {
		t.Fatal(err)
	}
	article := "---\ntitle: Test\npublished: 2026-07-25T00:00:00.000Z\ncategory: 思考\ntags: [自我成长]\n---\n\n正文\n"
	if err := os.WriteFile(filepath.Join(posts, "test.md"), []byte(article), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ProjectRootDir: root, LocalPostsDir: posts, WebhookSecret: "test-secret"}
	manager, err := jobs.NewManager(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewServer(cfg, manager).Register(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/taxonomy", nil)
	request.Header.Set("Authorization", "Bearer test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "自我成长") || !strings.Contains(response.Body.String(), "思考") {
		t.Fatalf("taxonomy was not seeded: %s", response.Body.String())
	}
}
