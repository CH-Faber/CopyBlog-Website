package media

import (
	"strings"
	"testing"
)

func testIndex() *Index {
	return NewIndex([]Object{
		{Key: "src/晚霞.jpg", ETag: `"etag-one"`},
		{Key: "src/Pasted image.png", ETag: "etag-two"},
		{Key: "other/duplicate.jpg", ETag: "etag-three"},
		{Key: "src/duplicate.jpg", ETag: "etag-four"},
	})
}

func TestRewriteDocument(t *testing.T) {
	input := "正文\n\n![[晚霞.jpg|回家路上的晚霞]]\n\n![[src/Pasted image.png|800]]\n\n![普通](src/晚霞.jpg)\n\n![远程](https://example.com/a.jpg)\n"
	output, assets, err := testIndex().RewriteDocument(input, "websites/article.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "![[") {
		t.Fatalf("wiki embed was not rewritten: %s", output)
	}
	if !strings.Contains(output, "![回家路上的晚霞](/obsidian-assets/") {
		t.Fatalf("alias was not preserved: %s", output)
	}
	if !strings.Contains(output, ` "回家路上的晚霞")`) {
		t.Fatalf("alias was not retained as a visible caption: %s", output)
	}
	if !strings.Contains(output, "![Pasted image](/obsidian-assets/") {
		t.Fatalf("numeric width should fall back to filename alt text: %s", output)
	}
	if !strings.Contains(output, "![远程](https://example.com/a.jpg)") {
		t.Fatalf("remote image changed: %s", output)
	}
	if len(assets) != 2 {
		t.Fatalf("expected two deduplicated assets, got %d", len(assets))
	}
}

func TestRewriteSkipsFencedCode(t *testing.T) {
	input := "```markdown\n![[晚霞.jpg]]\n```\n\n`![[晚霞.jpg]]`\n\n![[另一篇笔记]]\n\n![[晚霞.jpg]]"
	output, _, err := testIndex().RewriteDocument(input, "websites/article.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(output, "![[晚霞.jpg]]") != 2 {
		t.Fatalf("fenced and inline-code examples should remain unchanged: %s", output)
	}
	if !strings.Contains(output, "![[另一篇笔记]]") {
		t.Fatalf("non-image note embeds should remain unchanged: %s", output)
	}
}

func TestRewriteMarkdownDestinationWithTitle(t *testing.T) {
	input := `![晚霞](src/晚霞.jpg "标题")`
	output, assets, err := testIndex().RewriteDocument(input, "websites/article.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output, "![晚霞](/obsidian-assets/") || len(assets) != 1 {
		t.Fatalf("Markdown destination with a title was not rewritten: %s", output)
	}
	if !strings.Contains(output, ` "标题")`) {
		t.Fatalf("Markdown image title was not preserved: %s", output)
	}
}

func TestResolveRejectsMissingAmbiguousAndTraversal(t *testing.T) {
	for _, target := range []string{"missing.jpg", "duplicate.jpg", "../secret.jpg", "C:/outside.jpg"} {
		if _, err := testIndex().Resolve(target, "websites/article.md"); err == nil {
			t.Fatalf("expected %q to fail", target)
		}
	}
}

func TestPublicURLChangesWithETag(t *testing.T) {
	first, err := NewIndex([]Object{{Key: "src/a.jpg", ETag: "one"}}).Resolve("a.jpg", "websites/article.md")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewIndex([]Object{{Key: "src/a.jpg", ETag: "two"}}).Resolve("a.jpg", "websites/article.md")
	if err != nil {
		t.Fatal(err)
	}
	if first.PublicURL == second.PublicURL {
		t.Fatal("replacing an S3 object must produce a new cache-safe URL")
	}
}
