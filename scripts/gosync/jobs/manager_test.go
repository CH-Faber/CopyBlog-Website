package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gosync/config"
	"gosync/contentmodel"
	"gosync/taxonomy"
)

func TestUpdateArticleValidatesRevisionAndTags(t *testing.T) {
	root := t.TempDir()
	posts := filepath.Join(root, "src", "content", "posts")
	if err := os.MkdirAll(posts, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ProjectRootDir: root, LocalPostsDir: posts}
	values := &taxonomy.Taxonomy{Version: 1, Categories: []taxonomy.Category{{Name: "技术", Enabled: true}}, Tags: []taxonomy.ManagedTag{{Name: "已批准", Enabled: true, AISelectable: true}}}
	if err := taxonomy.SaveDraft(cfg, values); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.jobs["job"] = &Job{ID: "job", Articles: []*ArticleDraft{{ID: "article", Filename: "a.md", Revision: 2, Metadata: contentmodel.ArticleMetadata{Tags: []string{}}}}}

	_, err = manager.UpdateArticle("job", "article", UpdateArticleRequest{Revision: 1, Metadata: contentmodel.ArticleMetadata{Tags: []string{"已批准"}}})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	_, err = manager.UpdateArticle("job", "article", UpdateArticleRequest{Revision: 2, Metadata: contentmodel.ArticleMetadata{Tags: []string{"未批准"}}})
	if err == nil {
		t.Fatal("expected unknown tag to be rejected")
	}
	_, err = manager.UpdateArticle("job", "article", UpdateArticleRequest{Revision: 2, Metadata: contentmodel.ArticleMetadata{Category: "新分类", Tags: []string{"已批准"}}})
	if err == nil {
		t.Fatal("expected unknown category to be rejected")
	}
	updated, err := manager.UpdateArticle("job", "article", UpdateArticleRequest{Revision: 2, Metadata: contentmodel.ArticleMetadata{Title: "文章", Published: "2026-07-25T00:00:00.000Z", Category: "技术", Tags: []string{"已批准"}}, Content: "正文"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 3 || len(updated.Metadata.Tags) != 1 {
		t.Fatalf("unexpected update: %#v", updated)
	}
}

func TestValidateFinalTaxonomyStateOverlaysSelectedArticles(t *testing.T) {
	posts := t.TempDir()
	published := "---\ntitle: 旧文章\npublished: 2026-07-25T00:00:00.000Z\ncategory: 随笔\ntags: [旧标签]\n---\n\n正文\n"
	if err := os.WriteFile(filepath.Join(posts, "old.md"), []byte(published), 0644); err != nil {
		t.Fatal(err)
	}
	values := &taxonomy.Taxonomy{Version: 1, Categories: []taxonomy.Category{{Name: "随笔", Enabled: true}}, Tags: []taxonomy.ManagedTag{}}
	article := &ArticleDraft{ID: "article", Filename: "old.md", Metadata: contentmodel.ArticleMetadata{Title: "旧文章", Category: "随笔", Tags: []string{}}, Status: ArticleModified}
	if err := validateFinalTaxonomyState(posts, values, []*ArticleDraft{article}, map[string]PublishArticleRequest{}); err == nil {
		t.Fatal("expected unselected published article to keep blocking removed tag")
	}
	selected := map[string]PublishArticleRequest{"article": {ID: "article"}}
	if err := validateFinalTaxonomyState(posts, values, []*ArticleDraft{article}, selected); err != nil {
		t.Fatalf("selected article should remove the final tag reference: %v", err)
	}
}
