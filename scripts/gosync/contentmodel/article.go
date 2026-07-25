package contentmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DateString keeps dates as strings in the API while emitting an unquoted YAML timestamp.
type DateString string

func (d *DateString) UnmarshalYAML(node *yaml.Node) error {
	*d = DateString(node.Value)
	return nil
}

func (d DateString) MarshalYAML() (interface{}, error) {
	if strings.TrimSpace(string(d)) == "" {
		return "", nil
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: string(d)}, nil
}

type ArticleMetadata struct {
	Title       string                 `yaml:"title" json:"title"`
	Published   DateString             `yaml:"published" json:"published"`
	Updated     DateString             `yaml:"updated,omitempty" json:"updated,omitempty"`
	Draft       bool                   `yaml:"draft,omitempty" json:"draft,omitempty"`
	Description string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Image       string                 `yaml:"image,omitempty" json:"image,omitempty"`
	Tags        []string               `yaml:"tags,omitempty" json:"tags"`
	Category    string                 `yaml:"category,omitempty" json:"category,omitempty"`
	Lang        string                 `yaml:"lang,omitempty" json:"lang,omitempty"`
	Pinned      bool                   `yaml:"pinned,omitempty" json:"pinned,omitempty"`
	Encrypted   bool                   `yaml:"encrypted,omitempty" json:"encrypted,omitempty"`
	Password    string                 `yaml:"password,omitempty" json:"password,omitempty"`
	Disclaimer  interface{}            `yaml:"disclaimer,omitempty" json:"disclaimer,omitempty"`
	Extra       map[string]interface{} `yaml:",inline" json:"extra,omitempty"`
}

type Document struct {
	Metadata ArticleMetadata `json:"metadata"`
	Content  string          `json:"content"`
}

func Parse(raw string) (Document, error) {
	normalized := strings.ReplaceAll(strings.TrimPrefix(raw, "\ufeff"), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Document{Metadata: ArticleMetadata{Tags: []string{}}, Content: strings.TrimLeft(normalized, "\n")}, nil
	}

	closing := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closing = i
			break
		}
	}
	if closing == -1 {
		return Document{}, fmt.Errorf("frontmatter is missing its closing delimiter")
	}

	var metadata ArticleMetadata
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:closing], "\n")), &metadata); err != nil {
		return Document{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	if metadata.Tags == nil {
		metadata.Tags = []string{}
	}
	return Document{Metadata: metadata, Content: strings.TrimLeft(strings.Join(lines[closing+1:], "\n"), "\n")}, nil
}

func Serialize(doc Document) ([]byte, error) {
	if doc.Metadata.Tags == nil {
		doc.Metadata.Tags = []string{}
	}
	frontmatter, err := yaml.Marshal(doc.Metadata)
	if err != nil {
		return nil, fmt.Errorf("serialize frontmatter: %w", err)
	}
	return []byte("---\n" + strings.TrimSpace(string(frontmatter)) + "\n---\n\n" + strings.TrimSpace(doc.Content) + "\n"), nil
}

func EnsureDefaults(doc *Document, filename string, modified time.Time) {
	if strings.TrimSpace(doc.Metadata.Title) == "" {
		doc.Metadata.Title = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	if strings.TrimSpace(string(doc.Metadata.Published)) == "" {
		if modified.IsZero() {
			modified = time.Now()
		}
		doc.Metadata.Published = DateString(modified.UTC().Format("2006-01-02T15:04:05.000Z"))
	}
	if doc.Metadata.Tags == nil {
		doc.Metadata.Tags = []string{}
	}
}

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func HashDocument(doc Document) (string, error) {
	data, err := Serialize(doc)
	if err != nil {
		return "", err
	}
	return HashBytes(data), nil
}

// EquivalentDocuments compares parsed article meaning instead of raw Markdown
// bytes. This keeps harmless YAML formatting differences (for example an
// omitted empty tags field versus "tags:") from being reported as edits.
func EquivalentDocuments(left, right Document) (bool, error) {
	leftHash, err := HashDocument(left)
	if err != nil {
		return false, err
	}
	rightHash, err := HashDocument(right)
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}
