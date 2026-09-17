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

func TestTaxonomyUsageAPI(t *testing.T) {
	root := t.TempDir()
	posts := filepath.Join(root, "src", "content", "posts")
	if err := os.MkdirAll(posts, 0755); err != nil {
		t.Fatal(err)
	}
	article := "---\ntitle: Test\npublished: 2026-07-25T00:00:00.000Z\ncategory: 技术\ntags: [Astro, AI]\n---\n\nBody\n"
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

	request := httptest.NewRequest(http.MethodGet, "/api/v1/taxonomy/usage", nil)
	request.Header.Set("Authorization", "Bearer test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"Astro":1`) || !strings.Contains(response.Body.String(), `"AI":1`) || !strings.Contains(response.Body.String(), `"技术":1`) {
		t.Fatalf("unexpected usage: %s", response.Body.String())
	}
}

func TestSitePagesAPIReadsAndSavesDraft(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "src", "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	published := `{"version":1,"pages":[{"key":"home","name":"首页","title":"旧标题","description":"描述","heading":"文章","subtitle":"副标题"}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "site-pages.json"), []byte(published), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ProjectRootDir: root, LocalPostsDir: filepath.Join(root, "src", "content", "posts"), WebhookSecret: "test-secret"}
	manager, err := jobs.NewManager(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewServer(cfg, manager).Register(mux)

	body := `{"version":1,"pages":[{"key":"home","name":"首页","title":"新标题","description":"描述","heading":"近期文章","subtitle":"全部文章"}]}`
	request := httptest.NewRequest(http.MethodPut, "/api/v1/site-pages", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "新标题") {
		t.Fatalf("unexpected save response %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/site-pages", nil)
	request.Header.Set("Authorization", "Bearer test-secret")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "近期文章") {
		t.Fatalf("unexpected read response %d: %s", response.Code, response.Body.String())
	}
}
