package contentmodel

import (
	"strings"
	"testing"
	"time"
)

func TestParseSerializePreservesKnownAndExtraMetadata(t *testing.T) {
	raw := `---
title: "示例"
published: 2026-07-25T12:00:00.000Z
tags: ["技术", "Astro"]
customField: keep-me
---

# 正文
`
	doc, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Metadata.Title != "示例" || len(doc.Metadata.Tags) != 2 {
		t.Fatalf("unexpected metadata: %#v", doc.Metadata)
	}
	if doc.Metadata.Extra["customField"] != "keep-me" {
		t.Fatalf("custom field was not preserved: %#v", doc.Metadata.Extra)
	}
	serialized, err := Serialize(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(serialized), "customField: keep-me") || !strings.Contains(string(serialized), "published: 2026-07-25T12:00:00.000Z") {
		t.Fatalf("unexpected serialized document:\n%s", serialized)
	}
}

func TestEnsureDefaults(t *testing.T) {
	doc := Document{}
	EnsureDefaults(&doc, "文章.md", time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC))
	if doc.Metadata.Title != "文章" || doc.Metadata.Published == "" || doc.Metadata.Tags == nil {
		t.Fatalf("defaults missing: %#v", doc.Metadata)
	}
}

func TestParseRejectsUnclosedFrontmatter(t *testing.T) {
	if _, err := Parse("---\ntitle: broken\nbody"); err == nil {
		t.Fatal("expected an error for unclosed frontmatter")
	}
}

func TestEquivalentDocumentsIgnoresHarmlessMarkdownFormatting(t *testing.T) {
	leftRaw := "---\r\ntitle: Article\r\npublished: 2026-07-25T12:00:00.000Z\r\ndescription: Same\r\ntags:\r\ncategory: Notes\r\n---\r\n\r\nBody\r\n"
	rightRaw := "---\ntitle: \"Article\"\ncategory: Notes\ndescription: Same\npublished: 2026-07-25T12:00:00.000Z\n---\n\nBody\n\n"

	if HashBytes([]byte(leftRaw)) == HashBytes([]byte(rightRaw)) {
		t.Fatal("test inputs must have different raw hashes")
	}
	left, err := Parse(leftRaw)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Parse(rightRaw)
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := EquivalentDocuments(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if !equivalent {
		t.Fatal("semantically identical documents should be equivalent")
	}
}

func TestEquivalentDocumentsDetectsMeaningfulChanges(t *testing.T) {
	base, err := Parse("---\ntitle: Article\ntags: [one]\n---\n\nBody\n")
	if err != nil {
		t.Fatal(err)
	}

	changedTags, err := Parse("---\ntitle: Article\ntags: [one, two]\n---\n\nBody\n")
	if err != nil {
		t.Fatal(err)
	}
	changedBody, err := Parse("---\ntitle: Article\ntags: [one]\n---\n\nChanged body\n")
	if err != nil {
		t.Fatal(err)
	}

	for name, changed := range map[string]Document{"tags": changedTags, "body": changedBody} {
		equivalent, compareErr := EquivalentDocuments(base, changed)
		if compareErr != nil {
			t.Fatalf("%s comparison failed: %v", name, compareErr)
		}
		if equivalent {
			t.Fatalf("%s change must not be equivalent", name)
		}
	}
}
