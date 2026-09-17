package builder

import (
	"path/filepath"
	"testing"

	"gosync/config"
)

func TestApprovedPathAllowsOnlyExplicitContentRoots(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		ProjectRootDir:   root,
		LocalPostsDir:    filepath.Join(root, "src", "content", "posts"),
		LocalThoughtsDir: filepath.Join(root, "src", "content", "thoughts"),
	}

	allowed := []string{
		"article.md",
		ProjectFilePrefix + "src/data/content-taxonomy.json",
		ProjectFilePrefix + "src/data/site-pages.json",
		ProjectFilePrefix + "src/content/thoughts/flash.md",
		ProjectFilePrefix + "public/obsidian-assets/ab/image.jpg",
	}
	for _, name := range allowed {
		if _, _, err := approvedPath(cfg, name); err != nil {
			t.Fatalf("expected %q to be allowed: %v", name, err)
		}
	}

	rejected := []string{
		ProjectFilePrefix + "public/avatar.webp",
		ProjectFilePrefix + "scripts/gosync/main.go",
		ProjectFilePrefix + "../outside.jpg",
	}
	for _, name := range rejected {
		if _, _, err := approvedPath(cfg, name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}
